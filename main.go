// B2B E2E Integration Example (standalone, 无需 protoc 生成代码)
//
// 完整流程:
//  1. MM 推送订单簿深度 → MMHub
//  2. B2B 聚合器订阅平台 orderbook (QE WebSocket)
//  3. 收到深度推送，选择 MM
//  4. 调用 /v1/quote/firmQuote 指定 MM 下单
//  5. 解码返回的 rfq_quote_data → protobuf → 构建 on-chain 结构
//  6. ABI 编码 Settlement.settle() → approve tokenIn → 上链
//
// 依赖:
//
//	go get github.com/ethereum/go-ethereum
//	go get github.com/gorilla/websocket
//	go get github.com/tidwall/gjson
//
// 使用方式:
//
//	go run ./examples/b2b_e2e_example/
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
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
)

// ============================================================================
// 配置
// ============================================================================

const (
	// ==================== 以下配置需要用户自行填写 ====================

	// MMHub WebSocket 地址（MM 侧推送深度 + 接收询价）
	mmWsURL = "wss://<mmhub_domain>/ws"

	// B2B QE WebSocket 地址（聚合器侧订阅 orderbook）
	b2bWsURL = "wss://<qe_domain>/v1/quote/orderbook"

	// B2B 业务 API Key（从平台获取，用于 WebSocket 认证和 firmQuote 调用）
	businessApiKey = "<your_business_api_key>"

	// RFQ API 地址（用于触发 firmQuote）
	rfqAPIHost = "https://<rfq_api_domain>"

	// 链配置
	chainID = 56                     // BSC 主网，按需修改
	bscRPC  = "<your_rpc_endpoint>" // 例如 QuickNode / Alchemy / 自建节点

	// MM 认证信息（从平台获取）
	mmAuthToken  = "<your_mm_auth_token>"   // 平台分配的 MM JWT Token
	mmPrivateKey = "<your_mm_private_key>"  // MM signer 私钥（用于链上签名 + vault owner 操作），不含 0x 前缀
	mmVaultAddr  = "<your_vault_address>"   // 已部署的 MMVault 合约地址

	// 交易对配置（根据实际交易对修改）
	tokenA = "<token_a_address>" // 输入代币地址（例如 USDT: 0x55d398326f99059ff775485246999027b3197955）
	tokenB = "<token_b_address>" // 输出代币地址（例如 WBNB: 0xbb4cdb9cbd36b01bd1cbaebf2de08d9173bc095c）
	pairId = "<pair_id>"         // 交易对标识（例如 "USDT-WBNB"），需与平台注册的 pair_id 一致

	// 用于上链的测试账户私钥（发起 firmQuote + 执行交易的地址）
	// 注意: 这个账户需要持有 tokenA 余额并有足够原生代币作为 gas
	traderPrivateKey = "<your_trader_private_key>" // 不含 0x 前缀
)

// ============================================================================
// Protobuf 手动编解码 (无需 protoc，零外部依赖)
//
// 参考: https://protobuf.dev/programming-guides/encoding/
// Wire type 0 = varint, 2 = length-delimited (string/bytes/sub-message)
// ============================================================================

// --- 编码 helpers ---

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

// pbVarint 编码 varint 字段 (enum / uint64 / int64 / bool)
func pbVarint(fieldNum uint32, v uint64) []byte {
	if v == 0 {
		return nil // protobuf 默认值不编���
	}
	b := pbTag(fieldNum, 0)
	return pbAppendUvarint(b, v)
}

// pbBool 编码 bool 字段
func pbBool(fieldNum uint32, v bool) []byte {
	if !v {
		return nil
	}
	return pbVarint(fieldNum, 1)
}

// pbInt64 编码 int64 字段
func pbInt64(fieldNum uint32, v int64) []byte {
	if v == 0 {
		return nil
	}
	b := pbTag(fieldNum, 0)
	return pbAppendUvarint(b, uint64(v))
}

// pbString 编码 string 字段
func pbString(fieldNum uint32, s string) []byte {
	if s == "" {
		return nil
	}
	b := pbTag(fieldNum, 2)
	b = pbAppendUvarint(b, uint64(len(s)))
	return append(b, s...)
}

// pbBytes 编码 bytes 字段
func pbBytes(fieldNum uint32, data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	b := pbTag(fieldNum, 2)
	b = pbAppendUvarint(b, uint64(len(data)))
	return append(b, data...)
}

// pbMsg 构建 sub-message 字段 (把多个 field encoding 拼接后 length-prefix)
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

// pbEncode 拼接多个 field encoding
func pbEncode(fields ...[]byte) []byte {
	var b []byte
	for _, f := range fields {
		b = append(b, f...)
	}
	return b
}

// --- 解码 helpers ---

// pbDecoded 存储解码后的 protobuf 消息字段
type pbDecoded struct {
	varints  map[uint32]uint64   // varint 字段 (最后一个值)
	fields   map[uint32][]byte   // length-delimited 字段 (最后一个值)
	repeated map[uint32][][]byte // length-delimited repeated 字段 (所有值)
}

// pbDecode 解码 protobuf 二进制数据
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

// 所有 getter 方法都是 nil-safe 的
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
// Protobuf 消息类型常量 (对应 .proto 定义)
// ============================================================================

// MM Message.type 枚举值 (= oneof payload 字段号, 设计上一致)
//
//	参见 mm.proto: MessageType enum + Message.payload oneof
const (
	mmTypeDepthSnapshot uint64 = 3 // MESSAGE_TYPE_DEPTH_SNAPSHOT, field 3
	mmTypeQuoteRequest  uint64 = 4 // MESSAGE_TYPE_QUOTE_REQUEST, field 4
	mmTypeQuoteResponse uint64 = 5 // MESSAGE_TYPE_QUOTE_RESPONSE, field 5
	mmTypeHeartbeat     uint64 = 7 // MESSAGE_TYPE_HEARTBEAT, field 7
	mmTypeConnectionAck uint64 = 9 // MESSAGE_TYPE_CONNECTION_ACK, field 9
	mmQuoteStatusOK     uint64 = 1 // QUOTE_STATUS_SUCCESS
)

// QE QEMessage.type 枚举值
//
//	参见 quoteWs.proto: QEMessageType enum + QEMessage.payload oneof
//	注意: 枚举值和 oneof 字段号有 +2 偏移
const (
	qeTypeSubscribe     uint64 = 1 // QE_MESSAGE_TYPE_SUBSCRIBE
	qeTypeSubscribeAck  uint64 = 3 // QE_MESSAGE_TYPE_SUBSCRIBE_ACK
	qeTypeDepthUpdate   uint64 = 5 // QE_MESSAGE_TYPE_DEPTH_UPDATE
	qeTypeHeartbeat     uint64 = 6 // QE_MESSAGE_TYPE_HEARTBEAT
	qeTypeConnectionAck uint64 = 8 // QE_MESSAGE_TYPE_CONNECTION_ACK
)

// QE oneof 字段号 (= 枚举值 + 2)
const (
	qeFieldSubscribe     uint32 = 3  // SubscribeRequest
	qeFieldSubscribeAck  uint32 = 5  // SubscribeAck
	qeFieldDepthUpdate   uint32 = 7  // DepthUpdate
	qeFieldHeartbeat     uint32 = 8  // QEHeartbeat
	qeFieldConnectionAck uint32 = 10 // QEConnectionAck
)

// ============================================================================
// 通用数据结构
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
// Part A: MM 侧 — 连接 MMHub + 推送深度 + 响应询价
// ============================================================================

// MMClient 管理 MM 的 WebSocket 连接
type MMClient struct {
	conn      *websocket.Conn
	pk        *ecdsa.PrivateKey
	vaultAddr common.Address
	mu        sync.Mutex
	bids      []PriceLevel
	asks      []PriceLevel
}

// sendRaw 发送原始 protobuf 二进制消息
func (c *MMClient) sendRaw(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

// connectMM 建立 MM 到 MMHub 的 WebSocket 连接
func connectMM() (*MMClient, error) {
	log.Println("[MM] 连接 MMHub...")
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+mmAuthToken)

	conn, _, err := websocket.DefaultDialer.Dial(mmWsURL, headers)
	if err != nil {
		return nil, fmt.Errorf("WebSocket dial 失败: %w", err)
	}

	// 读取 ConnectionAck
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, data, err := conn.ReadMessage()
	conn.SetReadDeadline(time.Time{})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("读取 ConnectionAck 失败: %w", err)
	}

	// Message { type=9(CONNECTION_ACK), connection_ack(field 9) { success(1), session_id(2) } }
	d := pbDecode(data)
	ack := d.SubMsg(9) // connection_ack payload
	if ack == nil || !ack.Bool(1) {
		conn.Close()
		return nil, fmt.Errorf("ConnectionAck 失败")
	}
	log.Printf("[MM] 连接成功! SessionId=%s", ack.String(2))

	pk, _ := crypto.HexToECDSA(mmPrivateKey)
	return &MMClient{
		conn:      conn,
		pk:        pk,
		vaultAddr: common.HexToAddress(mmVaultAddr),
		bids:      []PriceLevel{{Price: "<bid_price>", Amount: "<bid_amount>"}}, // 买价和数量，根据交易对实际价格填写
		asks:      []PriceLevel{{Price: "<ask_price>", Amount: "<ask_amount>"}}, // 卖价和数量，根据交易对实际价格填写
	}, nil
}

// pushDepth 推送 DepthSnapshot
//
// DepthSnapshot { chain_id(1), pair_id(2), mm_id(3), token_a(4), token_b(5), bids(6), asks(7) }
// PriceLevel    { price(1), amount(2) }
func (c *MMClient) pushDepth() error {
	var depthFields [][]byte
	depthFields = append(depthFields,
		pbVarint(1, chainID),
		pbString(2, pairId),
		pbString(3, pairId), // mm_id
		pbString(4, tokenA),
		pbString(5, tokenB),
	)
	for _, b := range c.bids {
		depthFields = append(depthFields, pbMsg(6, pbString(1, b.Price), pbString(2, b.Amount)))
	}
	for _, a := range c.asks {
		depthFields = append(depthFields, pbMsg(7, pbString(1, a.Price), pbString(2, a.Amount)))
	}

	msg := pbEncode(
		pbVarint(1, mmTypeDepthSnapshot),   // type = DEPTH_SNAPSHOT (3)
		pbInt64(2, time.Now().UnixMilli()), // timestamp
		pbMsg(3, depthFields...),           // field 3 = depth_snapshot payload
	)
	return c.sendRaw(msg)
}

// mmMessageLoop 处理心跳和询价
func (c *MMClient) mmMessageLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("[MM] 读取失败: %v", err)
			return
		}
		d := pbDecode(data)
		msgType := d.Varint(1)

		switch msgType {
		case mmTypeHeartbeat: // 7
			// Heartbeat { ping(1), pong(2) }
			hb := d.SubMsg(7)
			if hb != nil && hb.Bool(1) { // ping
				pong := pbEncode(
					pbVarint(1, mmTypeHeartbeat),
					pbInt64(2, time.Now().UnixMilli()),
					pbMsg(7, pbBool(2, true)), // heartbeat.pong
				)
				c.sendRaw(pong)
			}
		case mmTypeQuoteRequest: // 4
			qr := d.SubMsg(4)
			if qr != nil {
				c.handleQuoteRequest(qr)
			}
		}
	}
}

// handleQuoteRequest 处理询价请求并回复报价
//
// QuoteRequest   { quote_id(1), chain_id(2), mm_id(3), protocol_version(4), quote_request_data(5) }
// QuoteRequestV1 { token_in(1), token_out(2), amount_in(3), executor(4), deadline(5), nonce(6) }
// MMQuoteV1      { maker(1), vault(2), executor(3), token_in(4), token_out(5), amount_in(6),
//
//	amount_out(7), deadline(8), nonce(9), cev(10), extra_data(11), mm_signature(12), quote_id(13) }
//
// QuoteResponse  { quote_id(1), chain_id(2), mm_id(3), status(4), protocol_version(5), mm_quote_data(6) }
func (c *MMClient) handleQuoteRequest(qr *pbDecoded) {
	quoteId := qr.String(1)
	qrChainId := qr.Uint64(2)
	mmId := qr.String(3)
	protocolVer := qr.String(4)
	quoteReqData := qr.Bytes(5)

	log.Printf("[MM] 收到询价: QuoteId=%s", quoteId)

	// 解码 QuoteRequestV1
	reqV1 := pbDecode(quoteReqData)
	if reqV1 == nil {
		log.Printf("[MM] 解析 QuoteRequestV1 失败")
		return
	}
	tIn := reqV1.String(1)      // token_in
	tOut := reqV1.String(2)     // token_out
	amtInStr := reqV1.String(3) // amount_in
	executor := reqV1.String(4) // executor
	deadline := reqV1.Int64(5)  // deadline
	nonceStr := reqV1.String(6) // nonce

	amountIn, _ := new(big.Int).SetString(amtInStr, 10)

	// 根据交易方向计算 amountOut（以下是示例，请根据实际交易对价格调整）
	// 例如 USDT→WBNB: amountOut = amountIn * bidPrice_numerator / bidPrice_denominator
	// 例如 WBNB→USDT: amountOut = amountIn * askPrice_numerator / askPrice_denominator
	var amountOut *big.Int
	if strings.EqualFold(tIn, tokenA) {
		// tokenA → tokenB: 使用 bid 价格
		// TODO: 根据实际交易对价格修改计算公式
		amountOut = new(big.Int).Mul(amountIn, big.NewInt(1685))
		amountOut.Div(amountOut, big.NewInt(1000000))
	} else {
		// tokenB → tokenA: 使用 ask 价格
		// TODO: 根据实际交易对价格修改计算公式
		amountOut = new(big.Int).Mul(amountIn, big.NewInt(587084319))
		amountOut.Div(amountOut, big.NewInt(1000000))
	}

	executorAddr := common.HexToAddress(executor)
	extraData := buildExtraData(executorAddr, common.HexToAddress(tIn))

	nonce, _ := new(big.Int).SetString(nonceStr, 10)
	if nonce == nil {
		nonce = big.NewInt(0)
	}
	makerAddr := crypto.PubkeyToAddress(c.pk.PublicKey)

	// EIP-712 签名
	sig := signMMQuote(c.pk, qrChainId, c.vaultAddr, &MMQuote{
		QuoteId: common.HexToHash(quoteId), Maker: makerAddr, Vault: c.vaultAddr,
		Executor: executorAddr, InputToken: common.HexToAddress(tIn),
		OutputToken: common.HexToAddress(tOut),
		AmountIn:    amountIn, AmountOut: amountOut,
		Deadline: big.NewInt(deadline), Nonce: nonce, ExtraData: extraData,
	})

	// 编码 MMQuoteV1
	mmQuoteData := pbEncode(
		pbString(1, makerAddr.Hex()),             // maker
		pbString(2, c.vaultAddr.Hex()),           // vault
		pbString(3, executor),                    // executor
		pbString(4, tIn),                         // token_in
		pbString(5, tOut),                        // token_out
		pbString(6, amountIn.String()),           // amount_in
		pbString(7, amountOut.String()),          // amount_out
		pbString(8, fmt.Sprintf("%d", deadline)), // deadline
		pbString(9, nonce.String()),              // nonce
		// field 10: confidence_extracted_value (默认，跳过)
		pbBytes(11, extraData), // extra_data
		pbBytes(12, sig),       // mm_signature
		pbString(13, quoteId),  // quote_id
	)

	// 编码 QuoteResponse
	respFields := [][]byte{
		pbString(1, quoteId),
		pbVarint(2, qrChainId),
		pbString(3, mmId),
		pbVarint(4, mmQuoteStatusOK), // status = SUCCESS
		pbString(5, protocolVer),
		pbBytes(6, mmQuoteData),
	}

	msg := pbEncode(
		pbVarint(1, mmTypeQuoteResponse), // type = QUOTE_RESPONSE (5)
		pbInt64(2, time.Now().UnixMilli()),
		pbMsg(5, respFields...), // field 5 = quote_response payload
	)
	c.sendRaw(msg)
	log.Printf("[MM] 已回复报价: maker=%s, amountOut=%s", makerAddr.Hex(), amountOut)
}

// ============================================================================
// Part B: B2B 聚合器侧 — 订阅 orderbook + 选择 MM + firmQuote + 上链
// ============================================================================

// B2BClient 管理聚合器到 QE 的 WebSocket 连接
type B2BClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

// connectB2B 建立 B2B WebSocket 连接
func connectB2B() (*B2BClient, error) {
	log.Println("[B2B] 连接 QE WebSocket...")
	fullURL := fmt.Sprintf("%s?api_key=%s", b2bWsURL, businessApiKey)

	dialer := websocket.Dialer{HandshakeTimeout: 30 * time.Second}
	conn, _, err := dialer.Dial(fullURL, http.Header{})
	if err != nil {
		return nil, fmt.Errorf("B2B dial 失败: %w", err)
	}

	// 读取 CONNECTION_ACK
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
		return nil, fmt.Errorf("B2B ConnectionAck 失败")
	}
	log.Printf("[B2B] 连接成功! SessionId=%s", ack.String(2))

	return &B2BClient{conn: conn}, nil
}

// subscribe 订阅交易对
//
// QEMessage { type=1(SUBSCRIBE), subscribe(field 3) { pairs(1) [{ chain_id(1), base_token(2), quote_token(3) }] } }
func (c *B2BClient) subscribe(chainId uint64, baseToken, quoteToken string) error {
	log.Printf("[B2B] 订阅: chainId=%d, base=%s, quote=%s", chainId, baseToken, quoteToken)

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

// waitForSubscribeAck 等待订阅确认
//
// SubscribeAck { success(1), statuses(2), error_message(3) }
func (c *B2BClient) waitForSubscribeAck(timeout time.Duration) error {
	c.conn.SetReadDeadline(time.Now().Add(timeout))
	defer c.conn.SetReadDeadline(time.Time{})

	_, data, err := c.conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("等待 SUBSCRIBE_ACK 超时: %w", err)
	}

	d := pbDecode(data)
	msgType := d.Varint(1)
	if msgType != qeTypeSubscribeAck {
		return fmt.Errorf("期望 SUBSCRIBE_ACK(3), 收到 type=%d", msgType)
	}
	subAck := d.SubMsg(qeFieldSubscribeAck) // field 5
	if subAck == nil || !subAck.Bool(1) {
		return fmt.Errorf("订阅失败")
	}
	statuses := subAck.RepeatedMsg(2)
	log.Printf("[B2B] 订阅成功! statuses=%d", len(statuses))
	return nil
}

// waitForDepthUpdate 等待深度推送
//
// DepthUpdate    { chain_id(1), mm_id(2), base_token(3), quote_token(4), bids(5), asks(6), update_time(7) }
// PriceLevelInfo { price(1), amount(2) }
// QEHeartbeat    { ping(1), pong(2) }
func (c *B2BClient) waitForDepthUpdate(timeout time.Duration) (*DepthUpdateInfo, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			continue
		}

		d := pbDecode(data)
		msgType := d.Varint(1)

		switch msgType {
		case qeTypeDepthUpdate: // 5
			du := d.SubMsg(qeFieldDepthUpdate) // field 7
			if du == nil {
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
	return nil, fmt.Errorf("等待深度推送超时 (%v)", timeout)
}

// ============================================================================
// Part C: B-side firmQuote API
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
	SettlementAddr  string // Settlement 合约地址
	RfqQuoteDataB64 string // base64 编码的 RFQQuoteV1 protobuf
}

func callBsideFirmQuote(mmId, tIn, tOut, amountIn, fromAddr string) (*BsideFirmQuoteResult, error) {
	log.Printf("[B2B] 调用 B-side firmQuote, mmId=%s...", mmId)

	req := &BsideFirmQuoteRequest{
		ChainId: chainID, MmId: mmId,
		TokenIn: tIn, TokenOut: tOut, AmountIn: amountIn,
		Deadline: time.Now().Unix() + 300, From: fromAddr, Recipient: fromAddr,
		ProtocolVersion: "v1",
	}

	body, _ := json.Marshal(req)
	url := rfqAPIHost + "/v1/quote/firmQuote"

	httpReq, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+businessApiKey)

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[B2B] firmQuote response: %s", string(respBody))

	result := gjson.ParseBytes(respBody)
	code := result.Get("code").Int()
	if code != 10000 {
		return nil, fmt.Errorf("firmQuote 失败: code=%d, msg=%s", code, result.Get("message").String())
	}

	data := result.Get("data")
	errorCode := data.Get("error_code").Int()
	if errorCode != 0 {
		return nil, fmt.Errorf("firmQuote error_code=%d: %s", errorCode, data.Get("error_message").String())
	}

	settlementAddr := data.Get("settlementAddress").String()
	if settlementAddr == "" {
		settlementAddr = data.Get("settlement_address").String()
	}
	rfqQuoteData := data.Get("rfqQuoteData").String()
	if rfqQuoteData == "" {
		rfqQuoteData = data.Get("rfq_quote_data").String()
	}

	log.Printf("[B2B] firmQuote 成功! settlement=%s", settlementAddr)
	return &BsideFirmQuoteResult{SettlementAddr: settlementAddr, RfqQuoteDataB64: rfqQuoteData}, nil
}

// ============================================================================
// Part D: 解码 RFQQuoteV1 → on-chain 结构 → ABI 编码 settle()
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

// decodeAndBuildSettleCalldata 解码 rfqQuoteData → 构建 settle() calldata
//
// RFQQuoteV1 { reserved(1), mm_quote(2), fee(3), rfq_signature(4) }
// MMQuoteV1  { maker(1), vault(2), executor(3), token_in(4), token_out(5), amount_in(6),
//
//	amount_out(7), deadline(8), nonce(9), cev(10), extra_data(11), mm_signature(12), quote_id(13) }
//
// FeeV1      { fee_to(1), fee_rate(2), fee_amount(3) }
// ConfidenceExtractedValue { T(1), N(2), M(3), E(4) }
func decodeAndBuildSettleCalldata(rfqQuoteDataB64 string, toAddr common.Address) ([]byte, *big.Int, error) {
	// 1. base64 解码
	rfqBytes, err := base64.StdEncoding.DecodeString(rfqQuoteDataB64)
	if err != nil {
		return nil, nil, fmt.Errorf("base64 解码失败: %w", err)
	}

	// 2. protobuf 反序列化 (手动解码)
	rfq := pbDecode(rfqBytes)
	mm := rfq.SubMsg(2)    // field 2 = mm_quote
	fee := rfq.SubMsg(3)   // field 3 = fee
	rfqSig := rfq.Bytes(4) // field 4 = rfq_signature

	log.Printf("[B2B] 解码 RFQQuoteV1:")
	log.Printf("  quote_id: %s", mm.String(13))
	log.Printf("  maker: %s, vault: %s, executor: %s", mm.String(1), mm.String(2), mm.String(3))
	log.Printf("  amountIn: %s, amountOut: %s", mm.String(6), mm.String(7))
	log.Printf("  mm_signature: %d bytes, rfq_signature: %d bytes", len(mm.Bytes(12)), len(rfqSig))

	// 3. 转换为 on-chain struct
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

	// 4. ABI 编码 settle(rfqQuote, amountIn, to)
	parsedABI, err := abi.JSON(strings.NewReader(settleABIJSON))
	if err != nil {
		return nil, nil, fmt.Errorf("解析 ABI 失败: %w", err)
	}

	amountIn := toBigInt(mm.String(6))
	callData, err := parsedABI.Pack("settle", onChainQuote, amountIn, toAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ABI 编码失败: %w", err)
	}

	log.Printf("[B2B] settle() calldata: %d bytes", len(callData))
	return callData, amountIn, nil
}

// ============================================================================
// Part E: EIP-712 签名 (MM 侧自包含)
// ============================================================================

type MMQuote struct {
	QuoteId                                              [32]byte
	Maker, Vault, Executor, InputToken, OutputToken      common.Address
	AmountIn, AmountOut, Deadline, Nonce                 *big.Int
	ConfidenceExtractedValueT, ConfidenceExtractedValueN *big.Int
	ConfidenceExtractedValueM, ConfidenceExtractedValueE *big.Int
	ExtraData                                            []byte
}

var mmQuoteTypeHash = crypto.Keccak256Hash([]byte(
	"MMQuote(bytes32 quoteId,address maker,address vault,address executor,address inputToken,address outputToken," +
		"uint256 amountIn,uint256 amountOut,uint256 deadline,uint256 nonce," +
		"uint256 confidenceExtractedValueT,uint256 confidenceExtractedValueN," +
		"uint256 confidenceExtractedValueM,uint256 confidenceExtractedValueE,bytes extraData)"))

func signMMQuote(pk *ecdsa.PrivateKey, chainId uint64, vault common.Address, q *MMQuote) []byte {
	// Domain Separator
	typeHash := crypto.Keccak256Hash([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"))
	nameHash := crypto.Keccak256Hash([]byte("RFQ MMVault"))
	verHash := crypto.Keccak256Hash([]byte("1"))

	b32, _ := abi.NewType("bytes32", "", nil)
	u256, _ := abi.NewType("uint256", "", nil)
	addr, _ := abi.NewType("address", "", nil)

	domArgs := abi.Arguments{{Type: b32}, {Type: b32}, {Type: b32}, {Type: u256}, {Type: addr}}
	domEnc, _ := domArgs.Pack(typeHash, nameHash, verHash, new(big.Int).SetUint64(chainId), vault)
	domSep := crypto.Keccak256(domEnc)

	// Struct Hash
	zero := big.NewInt(0)
	cevT, cevN, cevM, cevE := zero, zero, zero, zero
	if q.ConfidenceExtractedValueT != nil {
		cevT = q.ConfidenceExtractedValueT
	}
	if q.ConfidenceExtractedValueN != nil {
		cevN = q.ConfidenceExtractedValueN
	}
	if q.ConfidenceExtractedValueM != nil {
		cevM = q.ConfidenceExtractedValueM
	}
	if q.ConfidenceExtractedValueE != nil {
		cevE = q.ConfidenceExtractedValueE
	}

	structArgs := abi.Arguments{
		{Type: b32}, {Type: b32}, {Type: addr}, {Type: addr}, {Type: addr},
		{Type: addr}, {Type: addr}, {Type: u256}, {Type: u256}, {Type: u256},
		{Type: u256}, {Type: u256}, {Type: u256}, {Type: u256}, {Type: u256}, {Type: b32},
	}
	structEnc, _ := structArgs.Pack(
		mmQuoteTypeHash, q.QuoteId, q.Maker, q.Vault, q.Executor,
		q.InputToken, q.OutputToken, q.AmountIn, q.AmountOut, q.Deadline, q.Nonce,
		cevT, cevN, cevM, cevE, crypto.Keccak256Hash(q.ExtraData),
	)
	structHash := crypto.Keccak256(structEnc)

	digest := crypto.Keccak256(append([]byte{0x19, 0x01}, append(domSep, structHash...)...))
	sig, _ := crypto.Sign(digest, pk)
	if sig[64] < 27 {
		sig[64] += 27
	}
	return sig
}

func buildExtraData(executor, payToken common.Address) []byte {
	addrTy, _ := abi.NewType("address", "", nil)
	cbArgs := abi.Arguments{{Type: addrTy}}
	cb, _ := cbArgs.Pack(payToken)

	sqrtPrice := new(big.Int).Add(big.NewInt(4295128739), big.NewInt(1))
	boolTy, _ := abi.NewType("bool", "", nil)
	u160, _ := abi.NewType("uint160", "", nil)
	bytesTy, _ := abi.NewType("bytes", "", nil)
	args := abi.Arguments{{Type: addrTy}, {Type: boolTy}, {Type: u160}, {Type: bytesTy}}
	data, _ := args.Pack(executor, true, sqrtPrice, cb)
	return data
}

// ============================================================================
// Part F: 上链工具
// ============================================================================

var erc20ApproveSelector = crypto.Keccak256([]byte("approve(address,uint256)"))[:4]

func approveToken(client *ethclient.Client, pk *ecdsa.PrivateKey, tokenAddr, spenderAddr string) error {
	log.Printf("[链] Approve %s → %s", tokenAddr[:10]+"...", spenderAddr[:10]+"...")
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
	signed, err := types.SignTx(tx, types.NewEIP155Signer(big.NewInt(chainID)), pk)
	if err != nil {
		return fmt.Errorf("signTx: %w", err)
	}
	if err := client.SendTransaction(ctx, signed); err != nil {
		return fmt.Errorf("sendTx: %w", err)
	}
	log.Printf("[链] txHash=%s", signed.Hash().Hex())

	for i := 0; i < 60; i++ {
		receipt, err := client.TransactionReceipt(ctx, signed.Hash())
		if err == nil && receipt != nil {
			if receipt.Status == 1 {
				log.Printf("[链] 成功! gasUsed=%d", receipt.GasUsed)
				return nil
			}
			return fmt.Errorf("交易失败 status=0, txHash=%s", signed.Hash().Hex())
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("确认超时 60s, txHash=%s", signed.Hash().Hex())
}

// ============================================================================
// main
// ============================================================================

func main() {
	log.SetFlags(log.Ltime)
	log.Println("========== B2B E2E Integration Example ==========")
	log.Println("流程: MM推送深度 → B2B订阅orderbook → 选择MM → firmQuote → Settlement.settle() 上链")
	log.Println()

	traderPk, _ := crypto.HexToECDSA(traderPrivateKey)
	traderAddr := crypto.PubkeyToAddress(traderPk.PublicKey)
	log.Printf("Trader 地址: %s", traderAddr.Hex())

	// ====== Step 1: MM 连接 MMHub + 推送深度 ======
	mm, err := connectMM()
	if err != nil {
		log.Fatalf("MM 连接失败: %v", err)
	}
	defer mm.conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mm.mmMessageLoop(ctx)

	log.Println("[MM] 推送深度 5 轮...")
	for i := 0; i < 5; i++ {
		mm.pushDepth()
		time.Sleep(time.Second)
	}
	// 后台持续推送
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				mm.pushDepth()
				time.Sleep(2 * time.Second)
			}
		}
	}()

	// ====== Step 2: B2B 聚合器订阅 orderbook ======
	b2b, err := connectB2B()
	if err != nil {
		log.Fatalf("B2B 连接失败: %v", err)
	}
	defer b2b.conn.Close()

	if err := b2b.subscribe(chainID, tokenA, tokenB); err != nil {
		log.Fatalf("订阅失败: %v", err)
	}
	if err := b2b.waitForSubscribeAck(5 * time.Second); err != nil {
		log.Fatalf("订阅确认失败: %v", err)
	}

	// ====== Step 3: 等待深度推送 ======
	log.Println("[B2B] 等待深度推送...")
	mm.pushDepth()
	du, err := b2b.waitForDepthUpdate(15 * time.Second)
	if err != nil {
		log.Fatalf("等待深度失败: %v", err)
	}
	log.Printf("[B2B] 收到深度: mm_id=%s, base=%s, quote=%s, bids=%d, asks=%d",
		du.MmId, du.BaseToken, du.QuoteToken, len(du.Bids), len(du.Asks))
	if len(du.Bids) > 0 {
		log.Printf("[B2B]   最优买价: price=%s, amount=%s", du.Bids[0].Price, du.Bids[0].Amount)
	}

	// ====== Step 4: 选择 MM，调用 B-side firmQuote ======
	// 使用深度推送中的 mm_id（平台分配的 MM 标识），而非 signer address
	selectedMmId := du.MmId
	log.Printf("[B2B] 选择 MM: %s", selectedMmId)

	fqResult, err := callBsideFirmQuote(selectedMmId, tokenA, tokenB, "<amount_in>", traderAddr.Hex()) // 输入金额（最小单位），例如 1 USDT (18位) = "1000000000000000000"
	if err != nil {
		log.Fatalf("B-side firmQuote 失败: %v", err)
	}

	// ====== Step 5: 解码 RFQQuoteV1 → 构建 settle() calldata ======
	log.Println("[B2B] 解码 rfq_quote_data → 构建 settle() calldata...")
	settleCalldata, _, err := decodeAndBuildSettleCalldata(fqResult.RfqQuoteDataB64, traderAddr)
	if err != nil {
		log.Fatalf("构建 settle calldata 失败: %v", err)
	}

	// ====== Step 6: 上链 — approve + setAuthorizedSettlement + Settlement.settle() ======
	log.Println("[链] 上链执行...")
	ethClient, err := ethclient.Dial(bscRPC)
	if err != nil {
		log.Fatalf("连接 BSC 失败: %v", err)
	}
	defer ethClient.Close()

	// 6a. Vault owner (MM signer) 授权 Settlement 合约
	// setAuthorizedSettlement(address,bool) selector = 0x193f6ace
	mmPk, _ := crypto.HexToECDSA(mmPrivateKey)
	{
		addrTy, _ := abi.NewType("address", "", nil)
		boolTy, _ := abi.NewType("bool", "", nil)
		args := abi.Arguments{{Type: addrTy}, {Type: boolTy}}
		enc, _ := args.Pack(common.HexToAddress(fqResult.SettlementAddr), true)
		selector := crypto.Keccak256([]byte("setAuthorizedSettlement(address,bool)"))[:4]
		log.Printf("[链] Vault owner 授权 Settlement: %s", fqResult.SettlementAddr)
		if err := sendTx(ethClient, mmPk, common.HexToAddress(mmVaultAddr), big.NewInt(0), append(selector, enc...)); err != nil {
			log.Fatalf("setAuthorizedSettlement 失败: %v", err)
		}
	}

	if err := approveToken(ethClient, traderPk, tokenA, fqResult.SettlementAddr); err != nil {
		log.Fatalf("Approve 失败: %v", err)
	}

	log.Printf("[链] 调用 Settlement.settle() @ %s", fqResult.SettlementAddr)
	if err := sendTx(ethClient, traderPk, common.HexToAddress(fqResult.SettlementAddr), big.NewInt(0), settleCalldata); err != nil {
		log.Fatalf("Settlement.settle() 失败: %v", err)
	}

	log.Println()
	log.Println("========== B2B E2E 全流程完成! ==========")
	cancel()
	time.Sleep(time.Second)
}
