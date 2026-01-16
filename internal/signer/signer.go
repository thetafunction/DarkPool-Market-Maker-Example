package signer

import (
	"crypto/ecdsa"
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Signer is the EIP-712 signer interface
type Signer interface {
	// SignMMQuote signs an MMQuote using EIP-712 (with verifying contract domain)
	SignMMQuote(chainID uint64, quote *MMQuote) ([]byte, error)
	// GetAddress returns the signer address
	GetAddress() common.Address
}

// SignerConfig is the signer configuration
type SignerConfig struct {
	PrivateKey    string `json:"privateKey"`    // Private key (hexadecimal, highest priority)
	PrivateKeyEnv string `json:"privateKeyEnv"` // Private key environment variable name (fallback)
}

// signer is the signer implementation
type signer struct {
	privateKey    *ecdsa.PrivateKey
	address       common.Address
	domainManager *DomainManager
}

// NewSigner creates a signer
func NewSigner(privateKey *ecdsa.PrivateKey, domainManager *DomainManager) Signer {
	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	return &signer{
		privateKey:    privateKey,
		address:       address,
		domainManager: domainManager,
	}
}

// NewSignerFromHex creates a signer from hexadecimal private key
func NewSignerFromHex(hexKey string, domainManager *DomainManager) (Signer, error) {
	hexKey = strings.TrimPrefix(strings.TrimSpace(hexKey), "0x")
	privateKey, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}
	return NewSigner(privateKey, domainManager), nil
}

// NewSignerFromEnv creates a signer from environment variable
func NewSignerFromEnv(envName string, domainManager *DomainManager) (Signer, error) {
	hexKey := strings.TrimSpace(os.Getenv(envName))
	if hexKey == "" {
		return nil, fmt.Errorf("environment variable %s is not set", envName)
	}
	return NewSignerFromHex(hexKey, domainManager)
}

// NewSignerFromConfig creates a signer from config (prefers config file private key, falls back to environment variable)
func NewSignerFromConfig(config *SignerConfig, domainManager *DomainManager) (Signer, error) {
	var hexKey string

	// 1. Prefer private key from config file
	if config.PrivateKey != "" {
		hexKey = strings.TrimSpace(config.PrivateKey)
	} else if config.PrivateKeyEnv != "" {
		// 2. Read from environment variable
		hexKey = strings.TrimSpace(os.Getenv(config.PrivateKeyEnv))
		if hexKey == "" {
			return nil, fmt.Errorf("environment variable %s is not set and no privateKey in config", config.PrivateKeyEnv)
		}
	} else {
		return nil, fmt.Errorf("neither privateKey nor privateKeyEnv is configured")
	}

	return NewSignerFromHex(hexKey, domainManager)
}

// GetAddress returns the signer address
func (s *signer) GetAddress() common.Address {
	return s.address
}

// SignMMQuote signs an MMQuote using EIP-712 (with verifying contract domain)
func (s *signer) SignMMQuote(chainID uint64, quote *MMQuote) ([]byte, error) {
	// Get verifying contract domain separator
	domainSeparator, ok := s.domainManager.GetPoolDomainSeparator(chainID)
	if !ok {
		return nil, fmt.Errorf("RFQ Manager not configured for chainId %d", chainID)
	}

	// Calculate struct hash
	structHash, err := hashMMQuote(quote)
	if err != nil {
		return nil, fmt.Errorf("failed to hash MMQuote: %w", err)
	}

	// Calculate EIP-712 digest: keccak256("\x19\x01" || domainSeparator || structHash)
	digest := crypto.Keccak256Hash(
		[]byte{0x19, 0x01},
		domainSeparator,
		structHash,
	)

	// ECDSA signing
	sig, err := crypto.Sign(digest.Bytes(), s.privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	// Adjust v value to 27 or 28 (Ethereum standard)
	if sig[64] < 27 {
		sig[64] += 27
	}

	return sig, nil
}

// hashMMQuote calculates the struct hash of MMQuote
// Field order matches contract MMQUOTE_SIGNATURE_HASH
func hashMMQuote(quote *MMQuote) ([]byte, error) {
	// ABI encoding types
	bytes32Ty, _ := abi.NewType("bytes32", "", nil)
	addressTy, _ := abi.NewType("address", "", nil)
	uint256Ty, _ := abi.NewType("uint256", "", nil)

	// Build arguments (order matches contract MMQUOTE_SIGNATURE_HASH fields)
	args := abi.Arguments{
		{Type: bytes32Ty}, // typeHash
		{Type: addressTy}, // rfqManager
		{Type: addressTy}, // from
		{Type: addressTy}, // to
		{Type: addressTy}, // inputToken
		{Type: addressTy}, // outputToken
		{Type: uint256Ty}, // amountIn
		{Type: uint256Ty}, // amountOut
		{Type: uint256Ty}, // deadline
		{Type: uint256Ty}, // nonce
		{Type: bytes32Ty}, // extraDataHash
	}

	// Calculate keccak256 hash of extraData
	extraDataHash := crypto.Keccak256Hash(quote.ExtraData)

	// Pack encoding
	encoded, err := args.Pack(
		MMQuoteTypeHash,
		quote.RFQManager,
		quote.From,
		quote.To,
		quote.InputToken,
		quote.OutputToken,
		quote.AmountIn,
		quote.AmountOut,
		quote.Deadline,
		quote.Nonce,
		extraDataHash,
	)
	if err != nil {
		return nil, err
	}

	return crypto.Keccak256(encoded), nil
}

// HashExtraData calculates the keccak256 hash of extraData
func HashExtraData(extraData []byte) common.Hash {
	return crypto.Keccak256Hash(extraData)
}
