package signer

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestMMQuoteTypeHash(t *testing.T) {
	expected := crypto.Keccak256Hash([]byte(
		"MMQuote(bytes32 quoteId,address maker,address vault,address executor,address inputToken,address outputToken,uint256 amountIn,uint256 amountOut,uint256 deadline,uint256 nonce,uint256 confidenceExtractedValueT,uint256 confidenceExtractedValueN,uint256 confidenceExtractedValueM,uint256 confidenceExtractedValueE,bytes extraData)",
	))

	if MMQuoteTypeHash != expected {
		t.Errorf("MMQuoteTypeHash = %x, want %x", MMQuoteTypeHash, expected)
	}
}

func TestEIP712Domain_DomainSeparator(t *testing.T) {
	domain := &EIP712Domain{
		Name:              RFQVaultDomainName,
		Version:           RFQVaultDomainVersion,
		ChainID:           big.NewInt(56),
		VerifyingContract: common.HexToAddress("0x87e46572565Efd04121FB81CeB758c47168B4487"),
	}

	separator := domain.DomainSeparator()
	if len(separator) != 32 {
		t.Errorf("DomainSeparator length = %d, want 32", len(separator))
	}
	if string(separator) != string(domain.DomainSeparator()) {
		t.Error("DomainSeparator should be deterministic")
	}
}

func TestDomainManager(t *testing.T) {
	dm := NewDomainManager()
	vault := common.HexToAddress("0x87e46572565Efd04121FB81CeB758c47168B4487")
	dm.AddVaultDomain(56, vault)

	domain := dm.GetVaultDomain(56)
	if domain == nil {
		t.Fatal("GetVaultDomain returned nil")
	}
	if domain.Name != RFQVaultDomainName {
		t.Errorf("Domain.Name = %s, want %s", domain.Name, RFQVaultDomainName)
	}
	if domain.VerifyingContract != vault {
		t.Errorf("Domain.VerifyingContract = %s, want %s", domain.VerifyingContract.Hex(), vault.Hex())
	}
	if !dm.HasVaultDomain(56) {
		t.Error("HasVaultDomain(56) = false, want true")
	}
	if dm.HasVaultDomain(1) {
		t.Error("HasVaultDomain(1) = true, want false")
	}
	if _, ok := dm.GetVaultDomainSeparator(56); !ok {
		t.Error("GetVaultDomainSeparator(56) should succeed")
	}
	if _, ok := dm.GetVaultDomainSeparator(1); ok {
		t.Error("GetVaultDomainSeparator(1) should fail")
	}
}

func TestNewSignerFromHex(t *testing.T) {
	dm := NewDomainManager()
	dm.AddVaultDomain(56, common.HexToAddress("0x87e46572565Efd04121FB81CeB758c47168B4487"))

	validKey := "0x0000000000000000000000000000000000000000000000000000000000000001"
	s, err := NewSignerFromHex(validKey, dm)
	if err != nil {
		t.Fatalf("NewSignerFromHex failed: %v", err)
	}
	if s.GetAddress() == (common.Address{}) {
		t.Error("GetAddress returned zero address")
	}

	if _, err := NewSignerFromHex("invalid", dm); err == nil {
		t.Error("NewSignerFromHex should fail with invalid key")
	}
}

func TestSigner_SignMMQuote(t *testing.T) {
	dm := NewDomainManager()
	dm.AddVaultDomain(56, common.HexToAddress("0x87e46572565Efd04121FB81CeB758c47168B4487"))

	s, err := NewSignerFromHex("0x0000000000000000000000000000000000000000000000000000000000000001", dm)
	if err != nil {
		t.Fatalf("NewSignerFromHex failed: %v", err)
	}

	var quoteID [32]byte
	copy(quoteID[:], crypto.Keccak256([]byte("quote-id"))[:32])
	amountOut, ok := new(big.Int).SetString("600000000000000000000", 10)
	if !ok {
		t.Fatal("failed to parse amountOut")
	}
	quote := &MMQuote{
		QuoteId:                   quoteID,
		Maker:                     s.GetAddress(),
		Vault:                     common.HexToAddress("0x87e46572565Efd04121FB81CeB758c47168B4487"),
		Executor:                  common.HexToAddress("0xc4D38F584e998055Aef2c09bBe816465e6369993"),
		InputToken:                common.HexToAddress("0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c"),
		OutputToken:               common.HexToAddress("0x55d398326f99059fF775485246999027B3197955"),
		AmountIn:                  big.NewInt(1_000_000_000_000_000_000),
		AmountOut:                 amountOut,
		Deadline:                  big.NewInt(1_735_084_800),
		Nonce:                     big.NewInt(1),
		ConfidenceExtractedValueT: big.NewInt(30),
		ConfidenceExtractedValueN: big.NewInt(1_735_084_770),
		ConfidenceExtractedValueM: big.NewInt(50),
		ConfidenceExtractedValueE: big.NewInt(1_000_000_000_000_000_000),
		ExtraData:                 []byte{},
	}

	sig, err := s.SignMMQuote(56, quote)
	if err != nil {
		t.Fatalf("SignMMQuote failed: %v", err)
	}
	if len(sig) != 65 {
		t.Errorf("signature length = %d, want 65", len(sig))
	}
	if sig[64] != 27 && sig[64] != 28 {
		t.Errorf("signature v = %d, want 27 or 28", sig[64])
	}

	sig2, err := s.SignMMQuote(56, quote)
	if err != nil {
		t.Fatalf("SignMMQuote second call failed: %v", err)
	}
	if string(sig) != string(sig2) {
		t.Error("signature should be deterministic")
	}
}

func TestSigner_SignMMQuote_ChainNotConfigured(t *testing.T) {
	dm := NewDomainManager()
	s, err := NewSignerFromHex("0x0000000000000000000000000000000000000000000000000000000000000001", dm)
	if err != nil {
		t.Fatalf("NewSignerFromHex failed: %v", err)
	}

	if _, err := s.SignMMQuote(56, &MMQuote{}); err == nil {
		t.Error("SignMMQuote should fail when chain is not configured")
	}
}

func TestHashExtraData(t *testing.T) {
	hash := HashExtraData([]byte{0x01, 0x02, 0x03})
	if len(hash) != 32 {
		t.Errorf("HashExtraData length = %d, want 32", len(hash))
	}
	if hash != HashExtraData([]byte{0x01, 0x02, 0x03}) {
		t.Error("HashExtraData should be deterministic")
	}
}
