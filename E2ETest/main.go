// B2B E2E Integration Test (standalone, no protoc-generated code required)
//
// Test flow:
//  1. Connect to QE WebSocket, subscribe to a trading pair, receive orderbook depth
//  2. Pick an MM from the depth update, call B-side firmQuote API
//  3. Decode the returned rfq_quote_data -> protobuf -> build on-chain struct
//  4. ABI-encode Settlement.settle(), approve tokenIn, send transaction on-chain
//
// Dependencies:
//
//	go get github.com/ethereum/go-ethereum
//	go get github.com/gorilla/websocket
//	go get gopkg.in/yaml.v3
//
// Usage:
//
//	go run ./E2ETest/
package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v3"
)

// ============================================================================
// Configuration (loaded from config.yml)
// ============================================================================

type Config struct {
	B2BWsURL         string `yaml:"b2b_ws_url"`
	BusinessApiKey   string `yaml:"business_api_key"`
	RFQAPIHost       string `yaml:"rfq_api_host"`
	ChainID          uint64 `yaml:"chain_id"`
	RPCEndpoint      string `yaml:"rpc_endpoint"`
	TokenA           string `yaml:"token_a"`
	TokenB           string `yaml:"token_b"`
	PairID           string `yaml:"pair_id"`
	TargetMmId       string `yaml:"target_mm_id"`
	TraderPrivateKey string `yaml:"trader_private_key"`
	AmountIn         string `yaml:"amount_in"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// ============================================================================
// Protobuf manual encode/decode (no protoc, zero external dependency)
//
// Reference: https://protobuf.dev/programming-guides/encoding/
// Wire type 0 = varint, 2 = length-delimited (string/bytes/sub-message)
// ============================================================================

// --- Encoding helpers ---

func pbAppendUvarint(b []byte, v uint64) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

func pbTag(fieldNum, wireType uint32) []byte {
	return pbAppendUvarint(nil, uint64(fieldNum<<3|wireType))
}

// pbVarint encodes a varint field (enum / uint64 / int64 / bool).
func pbVarint(fieldNum uint32, v uint64) []byte {
	if v == 0 {
		return nil // protobuf default value is not encoded
	}
	b := pbTag(fieldNum, 0)
	return pbAppendUvarint(b, v)
}

// pbBool encodes a bool field.
func pbBool(fieldNum uint32, v bool) []byte {
	if !v {
		return nil
	}
	return pbVarint(fieldNum, 1)
}

// pbInt64 encodes an int64 field.
func pbInt64(fieldNum uint32, v int64) []byte {
	if v == 0 {
		return nil
	}
	b := pbTag(fieldNum, 0)
	return pbAppendUvarint(b, uint64(v))
}

// pbString encodes a string field.
func pbString(fieldNum uint32, s string) []byte {
	if s == "" {
		return nil
	}
	b := pbTag(fieldNum, 2)
	b = pbAppendUvarint(b, uint64(len(s)))
	return append(b, s...)
}

// pbBytes encodes a bytes field.
func pbBytes(fieldNum uint32, data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	b := pbTag(fieldNum, 2)
	b = pbAppendUvarint(b, uint64(len(data)))
	return append(b, data...)
}

// pbMsg builds a sub-message field (concatenates field encodings, then length-prefixes).
func pbMsg(fieldNum uint32, fields ...[]byte) []byte {
	var inner []byte
	for _, f := range fields {
		inner = append(inner, f...)
	}
	if len(inner) == 0 {
		return nil
	}
	b := pbTag(fieldNum, 2)
	b = pbAppendUvarint(b, uint64(len(inner)))
	return append(b, inner...)
}

// pbEncode concatenates multiple field encodings.
func pbEncode(fields ...[]byte) []byte {
	var b []byte
	for _, f := range fields {
		b = append(b, f...)
	}
	return b
}

// --- Decoding helpers ---

// pbDecoded stores decoded protobuf message fields.
type pbDecoded struct {
	varints  map[uint32]uint64   // varint fields (last value wins)
	fields   map[uint32][]byte   // length-delimited fields (last value wins)
	repeated map[uint32][][]byte // length-delimited repeated fields (all values)
}

// pbDecode decodes protobuf binary data.
func pbDecode(data []byte) *pbDecoded {
	d := &pbDecoded{
		varints:  make(map[uint32]uint64),
		fields:   make(map[uint32][]byte),
		repeated: make(map[uint32][][]byte),
	}
	for len(data) > 0 {
		tag, n := binary.Uvarint(data)
		if n <= 0 {
			break
		}
		data = data[n:]
		fieldNum := uint32(tag >> 3)
		wireType := tag & 0x7

		switch wireType {
		case 0: // varint
			v, n := binary.Uvarint(data)
			if n <= 0 {
				return d
			}
			data = data[n:]
			d.varints[fieldNum] = v
		case 2: // length-delimited
			length, n := binary.Uvarint(data)
			if n <= 0 {
				return d
			}
			data = data[n:]
			if uint64(len(data)) < length {
				return d
			}
			val := make([]byte, length)
			copy(val, data[:length])
			data = data[length:]
			d.fields[fieldNum] = val
			d.repeated[fieldNum] = append(d.repeated[fieldNum], val)
		case 1: // 64-bit fixed
			if len(data) < 8 {
				return d
			}
			data = data[8:]
		case 5: // 32-bit fixed
			if len(data) < 4 {
				return d
			}
			data = data[4:]
		}
	}
	return d
}

// All getter methods are nil-safe.
func (d *pbDecoded) Varint(num uint32) uint64 {
	if d == nil {
		return 0
	}
	return d.varints[num]
}
func (d *pbDecoded) Bool(num uint32) bool {
	if d == nil {
		return false
	}
	return d.varints[num] != 0
}
func (d *pbDecoded) Uint64(num uint32) uint64 {
	if d == nil {
		return 0
	}
	return d.varints[num]
}
func (d *pbDecoded) Int64(num uint32) int64 {
	if d == nil {
		return 0
	}
	return int64(d.varints[num])
}
func (d *pbDecoded) String(num uint32) string {
	if d == nil {
		return ""
	}
	if b, ok := d.fields[num]; ok {
		return string(b)
	}
	return ""
}
func (d *pbDecoded) Bytes(num uint32) []byte {
	if d == nil {
		return nil
	}
	return d.fields[num]
}
func (d *pbDecoded) SubMsg(num uint32) *pbDecoded {
	if d == nil {
		return nil
	}
	if b, ok := d.fields[num]; ok {
		return pbDecode(b)
	}
	return nil
}
func (d *pbDecoded) RepeatedMsg(num uint32) []*pbDecoded {
	if d == nil {
		return nil
	}
	var result []*pbDecoded
	for _, b := range d.repeated[num] {
		result = append(result, pbDecode(b))
	}
	return result
}

// ============================================================================
// QE protobuf message type constants (see quoteWs.proto)
// ============================================================================

// QEMessage.type enum values
const (
	qeTypeSubscribe     uint64 = 1 // QE_MESSAGE_TYPE_SUBSCRIBE
	qeTypeSubscribeAck  uint64 = 3 // QE_MESSAGE_TYPE_SUBSCRIBE_ACK
	qeTypeDepthUpdate   uint64 = 5 // QE_MESSAGE_TYPE_DEPTH_UPDATE
	qeTypeHeartbeat     uint64 = 6 // QE_MESSAGE_TYPE_HEARTBEAT
	qeTypeConnectionAck uint64 = 8 // QE_MESSAGE_TYPE_CONNECTION_ACK
)

// QE oneof field numbers (= enum value + 2)
const (
	qeFieldSubscribe     uint32 = 3  // SubscribeRequest
	qeFieldSubscribeAck  uint32 = 5  // SubscribeAck
	qeFieldDepthUpdate   uint32 = 7  // DepthUpdate
	qeFieldHeartbeat     uint32 = 8  // QEHeartbeat
	qeFieldConnectionAck uint32 = 10 // QEConnectionAck
)

// ============================================================================
// Common data structures
// ============================================================================

type PriceLevel struct {
	Price  string
	Amount string
}

type DepthUpdateInfo struct {
	MmId       string
	BaseToken  string
	QuoteToken string
	Bids       []PriceLevel
	Asks       []PriceLevel
}

// ============================================================================
// B2B Aggregator — QE WebSocket client: subscribe orderbook + receive depth
// ============================================================================

// B2BClient manages the aggregator-to-QE WebSocket connection.
type B2BClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

// connectB2B establishes the B2B WebSocket connection.
func connectB2B(cfg *Config) (*B2BClient, error) {
	log.Println("[B2B] Connecting to QE WebSocket...")
	fullURL := fmt.Sprintf("%s?api_key=%s", cfg.B2BWsURL, cfg.BusinessApiKey)

	dialer := websocket.Dialer{HandshakeTimeout: 30 * time.Second}
	conn, _, err := dialer.Dial(fullURL, http.Header{})
	if err != nil {
		return nil, fmt.Errorf("B2B dial failed: %w", err)
	}

	// Read CONNECTION_ACK
	// QEMessage { type=8(CONNECTION_ACK), connection_ack(field 10) { success(1), session_id(2) } }
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, data, err := conn.ReadMessage()
	conn.SetReadDeadline(time.Time{})
	if err != nil {
		conn.Close()
		return nil, err
	}

	d := pbDecode(data)
	ack := d.SubMsg(qeFieldConnectionAck) // field 10
	if ack == nil || !ack.Bool(1) {
		conn.Close()
		return nil, fmt.Errorf("B2B ConnectionAck failed")
	}
	log.Printf("[B2B] Connected! SessionId=%s", ack.String(2))

	return &B2BClient{conn: conn}, nil
}

// subscribe sends a pair subscription request.
//
// QEMessage { type=1(SUBSCRIBE), subscribe(field 3) { pairs(1) [{ chain_id(1), base_token(2), quote_token(3) }] } }
func (c *B2BClient) subscribe(chainId uint64, baseToken, quoteToken string) error {
	log.Printf("[B2B] Subscribing: chainId=%d, base=%s, quote=%s", chainId, baseToken, quoteToken)

	msg := pbEncode(
		pbVarint(1, qeTypeSubscribe), // type = SUBSCRIBE (1)
		pbInt64(2, time.Now().UnixMilli()),
		pbMsg(qeFieldSubscribe, // field 3 = subscribe payload
			pbMsg(1, // pairs[0] (PairSubscription)
				pbVarint(1, chainId),
				pbString(2, baseToken),
				pbString(3, quoteToken),
			),
		),
	)

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(websocket.BinaryMessage, msg)
}

// waitForSubscribeAck waits for the subscription acknowledgement.
//
// SubscribeAck { success(1), statuses(2), error_message(3) }
func (c *B2BClient) waitForSubscribeAck(timeout time.Duration) error {
	c.conn.SetReadDeadline(time.Now().Add(timeout))
	defer c.conn.SetReadDeadline(time.Time{})

	_, data, err := c.conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("waiting for SUBSCRIBE_ACK timed out: %w", err)
	}

	d := pbDecode(data)
	msgType := d.Varint(1)
	if msgType != qeTypeSubscribeAck {
		return fmt.Errorf("expected SUBSCRIBE_ACK(3), got type=%d", msgType)
	}
	subAck := d.SubMsg(qeFieldSubscribeAck) // field 5
	if subAck == nil || !subAck.Bool(1) {
		return fmt.Errorf("subscription failed")
	}
	statuses := subAck.RepeatedMsg(2)
	log.Printf("[B2B] Subscribed successfully! statuses=%d", len(statuses))
	return nil
}

// waitForDepthUpdate waits for a depth push from the QE.
//
// DepthUpdate    { chain_id(1), mm_id(2), base_token(3), quote_token(4), bids(5), asks(6), update_time(7) }
// PriceLevelInfo { price(1), amount(2) }
// QEHeartbeat    { ping(1), pong(2) }
func (c *B2BClient) waitForDepthUpdate(timeout time.Duration, targetMmId string) (*DepthUpdateInfo, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		c.conn.SetReadDeadline(time.Now().Add(remaining))
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
				break
			}
			return nil, fmt.Errorf("WebSocket read error: %w", err)
		}

		d := pbDecode(data)
		msgType := d.Varint(1)

		switch msgType {
		case qeTypeDepthUpdate: // 5
			du := d.SubMsg(qeFieldDepthUpdate) // field 7
			if du == nil {
				continue
			}
			mmId := du.String(2)
			if targetMmId != "" && mmId != targetMmId {
				log.Printf("[B2B] Skipping depth from mm_id=%s (waiting for %s)", mmId, targetMmId)
				continue
			}
			var bids, asks []PriceLevel
			for _, b := range du.RepeatedMsg(5) {
				bids = append(bids, PriceLevel{Price: b.String(1), Amount: b.String(2)})
			}
			for _, a := range du.RepeatedMsg(6) {
				asks = append(asks, PriceLevel{Price: a.String(1), Amount: a.String(2)})
			}
			return &DepthUpdateInfo{
				MmId: du.String(2), BaseToken: du.String(3), QuoteToken: du.String(4),
				Bids: bids, Asks: asks,
			}, nil

		case qeTypeHeartbeat: // 6
			hb := d.SubMsg(qeFieldHeartbeat) // field 8
			if hb != nil && hb.Bool(1) {     // ping
				pong := pbEncode(
					pbVarint(1, qeTypeHeartbeat),
					pbInt64(2, time.Now().UnixMilli()),
					pbMsg(qeFieldHeartbeat, pbBool(2, true)),
				)
				c.conn.WriteMessage(websocket.BinaryMessage, pong)
			}
		}
	}
	c.conn.SetReadDeadline(time.Time{})
	return nil, fmt.Errorf("waiting for depth update timed out (%v)", timeout)
}

// ============================================================================
// B-side firmQuote API
// ============================================================================

type BsideFirmQuoteRequest struct {
	ChainId         uint64 `json:"chainId"`
	MmId            string `json:"mmId"`
	TokenIn         string `json:"tokenIn"`
	TokenOut        string `json:"tokenOut"`
	AmountIn        string `json:"amountIn"`
	Deadline        int64  `json:"deadline"`
	From            string `json:"from"`
	Recipient       string `json:"recipient"`
	ProtocolVersion string `json:"protocolVersion"`
}

type BsideFirmQuoteResult struct {
	SettlementAddr  string // Settlement contract address
	RfqQuoteDataB64 string // base64-encoded RFQQuoteV1 protobuf
}

// firmQuoteResponse represents the JSON response from the firmQuote API.
type firmQuoteResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		ErrorCode         int    `json:"error_code"`
		ErrorMessage      string `json:"error_message"`
		SettlementAddress string `json:"settlement_address"`
		RfqQuoteData      string `json:"rfq_quote_data"`
	} `json:"data"`
}

func callBsideFirmQuote(cfg *Config, mmId, tIn, tOut, amountIn, fromAddr string) (*BsideFirmQuoteResult, error) {
	log.Printf("[B2B] Calling B-side firmQuote, mmId=%s...", mmId)

	req := &BsideFirmQuoteRequest{
		ChainId: cfg.ChainID, MmId: mmId,
		TokenIn: tIn, TokenOut: tOut, AmountIn: amountIn,
		Deadline: time.Now().Unix() + 300, From: fromAddr, Recipient: fromAddr,
		ProtocolVersion: "v1",
	}

	body, _ := json.Marshal(req)
	url := cfg.RFQAPIHost + "/v1/quote/firmQuote"

	httpReq, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.BusinessApiKey)

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[B2B] firmQuote response: %s", string(respBody))

	var result firmQuoteResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse firmQuote response: %w", err)
	}

	if result.Code != 10000 {
		return nil, fmt.Errorf("firmQuote failed: code=%d, msg=%s", result.Code, result.Message)
	}
	if result.Data.ErrorCode != 0 {
		return nil, fmt.Errorf("firmQuote error_code=%d: %s", result.Data.ErrorCode, result.Data.ErrorMessage)
	}

	settlementAddr := result.Data.SettlementAddress
	rfqQuoteData := result.Data.RfqQuoteData

	log.Printf("[B2B] firmQuote success! settlement=%s", settlementAddr)
	return &BsideFirmQuoteResult{SettlementAddr: settlementAddr, RfqQuoteDataB64: rfqQuoteData}, nil
}

// ============================================================================
// Decode RFQQuoteV1 -> on-chain struct -> ABI-encode settle()
// ============================================================================

// Settlement.settle() ABI
const settleABIJSON = `[{
  "type":"function","name":"settle",
  "inputs":[
    {"name":"rfqQuote","type":"tuple","components":[
      {"name":"mmQuote","type":"tuple","components":[
        {"name":"quoteId","type":"bytes32"},{"name":"maker","type":"address"},
        {"name":"vault","type":"address"},{"name":"executor","type":"address"},
        {"name":"inputToken","type":"address"},{"name":"outputToken","type":"address"},
        {"name":"amountIn","type":"uint256"},{"name":"amountOut","type":"uint256"},
        {"name":"deadline","type":"uint256"},{"name":"nonce","type":"uint256"},
        {"name":"confidenceExtractedValue","type":"tuple","components":[
          {"name":"confidenceExtractedValueT","type":"uint256"},
          {"name":"confidenceExtractedValueN","type":"uint256"},
          {"name":"confidenceExtractedValueM","type":"uint256"},
          {"name":"confidenceExtractedValueE","type":"uint256"}
        ]},
        {"name":"extraData","type":"bytes"},{"name":"mmSignature","type":"bytes"}
      ]},
      {"name":"fee","type":"tuple","components":[
        {"name":"feeTo","type":"address"},{"name":"feeRate","type":"uint256"}
      ]},
      {"name":"rfqSignature","type":"bytes"}
    ]},
    {"name":"amountIn","type":"uint256"},{"name":"to","type":"address"}
  ],
  "outputs":[{"name":"amountOut","type":"uint256"},{"name":"feeAmount","type":"uint256"}],
  "stateMutability":"nonpayable"
}]`

// On-chain structs
type OnChainCEV struct {
	ConfidenceExtractedValueT *big.Int
	ConfidenceExtractedValueN *big.Int
	ConfidenceExtractedValueM *big.Int
	ConfidenceExtractedValueE *big.Int
}
type OnChainMMQuote struct {
	QuoteId                  [32]byte
	Maker, Vault, Executor   common.Address
	InputToken, OutputToken  common.Address
	AmountIn, AmountOut      *big.Int
	Deadline, Nonce          *big.Int
	ConfidenceExtractedValue OnChainCEV
	ExtraData                []byte
	MmSignature              []byte
}
type OnChainFee struct {
	FeeTo   common.Address
	FeeRate *big.Int
}
type OnChainRFQQuote struct {
	MmQuote      OnChainMMQuote
	Fee          OnChainFee
	RfqSignature []byte
}

func toBigInt(s string) *big.Int {
	if s == "" {
		return big.NewInt(0)
	}
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		s = strings.TrimPrefix(s, "0x")
		n, _ = new(big.Int).SetString(s, 16)
		if n == nil {
			return big.NewInt(0)
		}
	}
	return n
}

func hexToBytes32(s string) [32]byte {
	s = strings.TrimPrefix(s, "0x")
	b, _ := hex.DecodeString(s)
	var result [32]byte
	copy(result[len(result)-len(b):], b)
	return result
}

// decodeAndBuildSettleCalldata decodes rfqQuoteData and builds settle() calldata.
//
// RFQQuoteV1 top-level: { reserved(1), mm_quote(2: sub-msg), fee(3: sub-msg), rfq_signature(4: bytes) }
// The mm_quote sub-message uses MMQuoteV1 field layout:
//   maker(1), vault(2), executor(3), token_in(4), token_out(5), amount_in(6),
//   amount_out(7), deadline(8), nonce(9), cev(10), extra_data(11), mm_signature(12), quote_id(13)
// FeeV1      { fee_to(1), fee_rate(2), fee_amount(3) }
// ConfidenceExtractedValue { T(1), N(2), M(3), E(4) }
func decodeAndBuildSettleCalldata(rfqQuoteDataB64 string, toAddr common.Address) ([]byte, *big.Int, error) {
	// 1. Base64 decode
	rfqBytes, err := base64.StdEncoding.DecodeString(rfqQuoteDataB64)
	if err != nil {
		return nil, nil, fmt.Errorf("base64 decode failed: %w", err)
	}

	// 2. Protobuf deserialization (manual decode)
	rfq := pbDecode(rfqBytes)

	// RFQQuoteV1 { reserved(1), mm_quote(2), fee(3), rfq_signature(4) }
	mm := rfq.SubMsg(2) // field 2 = mm_quote
	if mm.String(1) == "" {
		// Fallback: mm_quote fields are embedded directly in the top-level message
		mm = rfq
	}
	fee := rfq.SubMsg(3)   // field 3 = fee
	rfqSig := rfq.Bytes(4) // field 4 = rfq_signature

	log.Printf("[B2B] Decoded RFQQuoteV1:")
	log.Printf("  quote_id: %s", mm.String(13))
	log.Printf("  maker: %s, vault: %s, executor: %s", mm.String(1), mm.String(2), mm.String(3))
	log.Printf("  amountIn: %s, amountOut: %s", mm.String(6), mm.String(7))
	log.Printf("  mm_signature: %d bytes, rfq_signature: %d bytes", len(mm.Bytes(12)), len(rfqSig))

	// 3. Convert to on-chain struct
	cev := OnChainCEV{big.NewInt(0), big.NewInt(0), big.NewInt(0), big.NewInt(0)}
	if cevMsg := mm.SubMsg(10); cevMsg != nil {
		cev.ConfidenceExtractedValueT = toBigInt(cevMsg.String(1))
		cev.ConfidenceExtractedValueN = toBigInt(cevMsg.String(2))
		cev.ConfidenceExtractedValueM = toBigInt(cevMsg.String(3))
		cev.ConfidenceExtractedValueE = toBigInt(cevMsg.String(4))
	}

	feeTo, feeRate := common.Address{}, big.NewInt(0)
	if fee != nil {
		feeTo = common.HexToAddress(fee.String(1))
		feeRate = toBigInt(fee.String(2))
	}

	onChainQuote := OnChainRFQQuote{
		MmQuote: OnChainMMQuote{
			QuoteId: hexToBytes32(mm.String(13)), Maker: common.HexToAddress(mm.String(1)),
			Vault: common.HexToAddress(mm.String(2)), Executor: common.HexToAddress(mm.String(3)),
			InputToken: common.HexToAddress(mm.String(4)), OutputToken: common.HexToAddress(mm.String(5)),
			AmountIn: toBigInt(mm.String(6)), AmountOut: toBigInt(mm.String(7)),
			Deadline: toBigInt(mm.String(8)), Nonce: toBigInt(mm.String(9)),
			ConfidenceExtractedValue: cev,
			ExtraData:                mm.Bytes(11), MmSignature: mm.Bytes(12),
		},
		Fee:          OnChainFee{FeeTo: feeTo, FeeRate: feeRate},
		RfqSignature: rfqSig,
	}

	// 4. ABI-encode settle(rfqQuote, amountIn, to)
	parsedABI, err := abi.JSON(strings.NewReader(settleABIJSON))
	if err != nil {
		return nil, nil, fmt.Errorf("parse ABI failed: %w", err)
	}

	amountIn := toBigInt(mm.String(6))
	callData, err := parsedABI.Pack("settle", onChainQuote, amountIn, toAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ABI encode failed: %w", err)
	}

	log.Printf("[B2B] settle() calldata: %d bytes", len(callData))
	return callData, amountIn, nil
}

// ============================================================================
// On-chain utilities
// ============================================================================

var erc20ApproveSelector = crypto.Keccak256([]byte("approve(address,uint256)"))[:4]

func approveToken(client *ethclient.Client, pk *ecdsa.PrivateKey, tokenAddr, spenderAddr string) error {
	log.Printf("[Chain] Approve %s -> %s", tokenAddr[:10]+"...", spenderAddr[:10]+"...")
	addrTy, _ := abi.NewType("address", "", nil)
	uint256Ty, _ := abi.NewType("uint256", "", nil)
	args := abi.Arguments{{Type: addrTy}, {Type: uint256Ty}}
	maxU256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	enc, _ := args.Pack(common.HexToAddress(spenderAddr), maxU256)
	return sendTx(client, pk, common.HexToAddress(tokenAddr), big.NewInt(0), append(erc20ApproveSelector, enc...))
}

func sendTx(client *ethclient.Client, pk *ecdsa.PrivateKey, to common.Address, value *big.Int, data []byte) error {
	ctx := context.Background()
	from := crypto.PubkeyToAddress(pk.PublicKey)
	nonce, err := client.PendingNonceAt(ctx, from)
	if err != nil {
		return fmt.Errorf("nonce: %w", err)
	}
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return fmt.Errorf("gasPrice: %w", err)
	}

	tx := types.NewTransaction(nonce, to, value, 500000, gasPrice, data)
	signed, err := types.SignTx(tx, types.NewEIP155Signer(big.NewInt(int64(chainIDFromPk(client)))), pk)
	if err != nil {
		return fmt.Errorf("signTx: %w", err)
	}
	if err := client.SendTransaction(ctx, signed); err != nil {
		return fmt.Errorf("sendTx: %w", err)
	}
	log.Printf("[Chain] txHash=%s", signed.Hash().Hex())

	for i := 0; i < 60; i++ {
		receipt, err := client.TransactionReceipt(ctx, signed.Hash())
		if err == nil && receipt != nil {
			if receipt.Status == 1 {
				log.Printf("[Chain] Success! gasUsed=%d", receipt.GasUsed)
				return nil
			}
			return fmt.Errorf("transaction failed status=0, txHash=%s", signed.Hash().Hex())
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("confirmation timeout 60s, txHash=%s", signed.Hash().Hex())
}

// chainIDFromPk fetches the chain ID from the connected node.
func chainIDFromPk(client *ethclient.Client) uint64 {
	cid, err := client.ChainID(context.Background())
	if err != nil {
		return 0
	}
	return cid.Uint64()
}

// ============================================================================
// main
// ============================================================================

func main() {
	log.SetFlags(log.Ltime)
	log.Println("========== B2B E2E Integration Test ==========")
	log.Println("Flow: Subscribe orderbook -> Verify depth -> firmQuote -> Settlement.settle() on-chain")
	log.Println()

	// Load configuration
	cfgPath := "E2ETest/config.yml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	traderPk, err := crypto.HexToECDSA(cfg.TraderPrivateKey)
	if err != nil {
		log.Fatalf("Invalid trader private key: %v", err)
	}
	traderAddr := crypto.PubkeyToAddress(traderPk.PublicKey)
	log.Printf("Trader address: %s", traderAddr.Hex())

	// ====== Step 1: Subscribe to orderbook and verify depth is available ======
	log.Println()
	log.Println("====== Step 1: Subscribe to orderbook and verify depth ======")

	b2b, err := connectB2B(cfg)
	if err != nil {
		log.Fatalf("B2B connection failed: %v", err)
	}
	defer b2b.conn.Close()

	if err := b2b.subscribe(cfg.ChainID, cfg.TokenA, cfg.TokenB); err != nil {
		log.Fatalf("Subscribe failed: %v", err)
	}
	if err := b2b.waitForSubscribeAck(5 * time.Second); err != nil {
		log.Fatalf("Subscribe ack failed: %v", err)
	}

	log.Println("[B2B] Waiting for depth update from live MM...")
	if cfg.TargetMmId != "" {
		log.Printf("[B2B] Filtering for target mm_id=%s", cfg.TargetMmId)
	}
	du, err := b2b.waitForDepthUpdate(30*time.Second, cfg.TargetMmId)
	if err != nil {
		log.Fatalf("Depth update failed: %v", err)
	}
	log.Printf("[B2B] Depth received: mm_id=%s, base=%s, quote=%s, bids=%d, asks=%d",
		du.MmId, du.BaseToken, du.QuoteToken, len(du.Bids), len(du.Asks))
	if len(du.Bids) > 0 {
		log.Printf("[B2B]   Best bid: price=%s, amount=%s", du.Bids[0].Price, du.Bids[0].Amount)
	}
	if len(du.Asks) > 0 {
		log.Printf("[B2B]   Best ask: price=%s, amount=%s", du.Asks[0].Price, du.Asks[0].Amount)
	}
	log.Println("[Step 1] PASSED: Orderbook depth received successfully")

	// ====== Step 2: Pick MM and call B-side firmQuote ======
	log.Println()
	log.Println("====== Step 2: Call firmQuote and verify response ======")

	selectedMmId := du.MmId
	log.Printf("[B2B] Selected MM: %s", selectedMmId)

	fqResult, err := callBsideFirmQuote(cfg, selectedMmId, cfg.TokenA, cfg.TokenB, cfg.AmountIn, traderAddr.Hex())
	if err != nil {
		log.Fatalf("B-side firmQuote failed: %v", err)
	}
	log.Println("[Step 2] PASSED: firmQuote returned valid result")

	// ====== Step 3: Decode RFQQuoteV1, approve, and settle on-chain ======
	log.Println()
	log.Println("====== Step 3: Decode quote and settle on-chain ======")

	log.Println("[B2B] Decoding rfq_quote_data -> building settle() calldata...")
	settleCalldata, _, err := decodeAndBuildSettleCalldata(fqResult.RfqQuoteDataB64, traderAddr)
	if err != nil {
		log.Fatalf("Build settle calldata failed: %v", err)
	}

	log.Println("[Chain] Executing on-chain transactions...")
	ethClient, err := ethclient.Dial(cfg.RPCEndpoint)
	if err != nil {
		log.Fatalf("Connect to chain failed: %v", err)
	}
	defer ethClient.Close()

	// Approve tokenIn for Settlement contract
	if err := approveToken(ethClient, traderPk, cfg.TokenA, fqResult.SettlementAddr); err != nil {
		log.Fatalf("Approve failed: %v", err)
	}

	// Call Settlement.settle()
	log.Printf("[Chain] Calling Settlement.settle() @ %s", fqResult.SettlementAddr)
	if err := sendTx(ethClient, traderPk, common.HexToAddress(fqResult.SettlementAddr), big.NewInt(0), settleCalldata); err != nil {
		log.Fatalf("Settlement.settle() failed: %v", err)
	}
	log.Println("[Step 3] PASSED: On-chain settlement succeeded")

	log.Println()
	log.Println("========== B2B E2E Test PASSED ==========")
}
