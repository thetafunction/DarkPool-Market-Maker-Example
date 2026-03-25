package signer

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type Signer interface {
	SignMMQuote(chainID uint64, quote *MMQuote) ([]byte, error)
	GetAddress() common.Address
}

type SignerConfig struct {
	PrivateKey    string `json:"privateKey"`
	PrivateKeyEnv string `json:"privateKeyEnv"`
}

type signer struct {
	privateKey    *ecdsa.PrivateKey
	address       common.Address
	domainManager *DomainManager
}

func NewSigner(privateKey *ecdsa.PrivateKey, domainManager *DomainManager) Signer {
	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	return &signer{
		privateKey:    privateKey,
		address:       address,
		domainManager: domainManager,
	}
}

func NewSignerFromHex(hexKey string, domainManager *DomainManager) (Signer, error) {
	hexKey = strings.TrimPrefix(strings.TrimSpace(hexKey), "0x")
	privateKey, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}
	return NewSigner(privateKey, domainManager), nil
}

func NewSignerFromEnv(envName string, domainManager *DomainManager) (Signer, error) {
	hexKey := strings.TrimSpace(os.Getenv(envName))
	if hexKey == "" {
		return nil, fmt.Errorf("environment variable %s is not set", envName)
	}
	return NewSignerFromHex(hexKey, domainManager)
}

func NewSignerFromConfig(config *SignerConfig, domainManager *DomainManager) (Signer, error) {
	var hexKey string
	if config.PrivateKey != "" {
		hexKey = strings.TrimSpace(config.PrivateKey)
	} else if config.PrivateKeyEnv != "" {
		hexKey = strings.TrimSpace(os.Getenv(config.PrivateKeyEnv))
		if hexKey == "" {
			return nil, fmt.Errorf("environment variable %s is not set and no privateKey in config", config.PrivateKeyEnv)
		}
	} else {
		return nil, fmt.Errorf("neither privateKey nor privateKeyEnv is configured")
	}

	return NewSignerFromHex(hexKey, domainManager)
}

func (s *signer) GetAddress() common.Address {
	return s.address
}

func (s *signer) SignMMQuote(chainID uint64, quote *MMQuote) ([]byte, error) {
	domainSeparator, ok := s.domainManager.GetVaultDomainSeparator(chainID)
	if !ok {
		return nil, fmt.Errorf("vault domain not configured for chainId %d", chainID)
	}

	structHash, err := hashMMQuote(quote)
	if err != nil {
		return nil, fmt.Errorf("failed to hash MMQuote: %w", err)
	}

	digest := crypto.Keccak256Hash(
		[]byte{0x19, 0x01},
		domainSeparator,
		structHash,
	)

	sig, err := crypto.Sign(digest.Bytes(), s.privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	if sig[64] < 27 {
		sig[64] += 27
	}

	return sig, nil
}

func hashMMQuote(quote *MMQuote) ([]byte, error) {
	bytes32Ty, _ := abi.NewType("bytes32", "", nil)
	addressTy, _ := abi.NewType("address", "", nil)
	uint256Ty, _ := abi.NewType("uint256", "", nil)

	args := abi.Arguments{
		{Type: bytes32Ty},
		{Type: bytes32Ty},
		{Type: addressTy},
		{Type: addressTy},
		{Type: addressTy},
		{Type: addressTy},
		{Type: addressTy},
		{Type: uint256Ty},
		{Type: uint256Ty},
		{Type: uint256Ty},
		{Type: uint256Ty},
		{Type: uint256Ty},
		{Type: uint256Ty},
		{Type: uint256Ty},
		{Type: uint256Ty},
		{Type: bytes32Ty},
	}

	encoded, err := args.Pack(
		MMQuoteTypeHash,
		quote.QuoteId,
		quote.Maker,
		quote.Vault,
		quote.Executor,
		quote.InputToken,
		quote.OutputToken,
		quote.AmountIn,
		quote.AmountOut,
		quote.Deadline,
		quote.Nonce,
		orZero(quote.ConfidenceExtractedValueT),
		orZero(quote.ConfidenceExtractedValueN),
		orZero(quote.ConfidenceExtractedValueM),
		orZero(quote.ConfidenceExtractedValueE),
		crypto.Keccak256Hash(quote.ExtraData),
	)
	if err != nil {
		return nil, err
	}

	return crypto.Keccak256(encoded), nil
}

func orZero(v *big.Int) *big.Int {
	if v == nil {
		return big.NewInt(0)
	}
	return v
}

func HashExtraData(extraData []byte) common.Hash {
	return crypto.Keccak256Hash(extraData)
}
