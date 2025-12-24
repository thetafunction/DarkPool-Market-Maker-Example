package signer

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestMMQuoteTypeHash(t *testing.T) {
	// Verify TypeHash calculation is correct
	expected := crypto.Keccak256Hash([]byte(
		"MMQuote(address pool,address from,address to,address inputToken,address outputToken,uint256 amountIn,uint256 amountOut,uint256 deadline,uint256 nonce,bytes32 extraDataHash)",
	))

	if MMQuoteTypeHash != expected {
		t.Errorf("MMQuoteTypeHash = %x, want %x", MMQuoteTypeHash, expected)
	}
}

func TestEIP712Domain_DomainSeparator(t *testing.T) {
	domain := &EIP712Domain{
		Name:              "RFQ Pool",
		Version:           "1",
		ChainID:           big.NewInt(56),
		VerifyingContract: common.HexToAddress("0x28D3a265f6d40867986004029ee91F4C9532fCC5"),
	}

	separator := domain.DomainSeparator()
	if len(separator) != 32 {
		t.Errorf("DomainSeparator length = %d, want 32", len(separator))
	}

	// Ensure two calls return the same result
	separator2 := domain.DomainSeparator()
	if string(separator) != string(separator2) {
		t.Error("DomainSeparator should be deterministic")
	}
}

func TestDomainManager(t *testing.T) {
	dm := NewDomainManager()

	// Test adding domain
	dm.AddPoolDomain(56, common.HexToAddress("0x28D3a265f6d40867986004029ee91F4C9532fCC5"))

	// Test getting domain
	domain := dm.GetPoolDomain(56)
	if domain == nil {
		t.Fatal("GetPoolDomain returned nil")
	}
	if domain.Name != DefaultDomainName {
		t.Errorf("Domain.Name = %s, want %s", domain.Name, DefaultDomainName)
	}
	if domain.ChainID.Int64() != 56 {
		t.Errorf("Domain.ChainID = %d, want 56", domain.ChainID.Int64())
	}

	// Test HasPoolDomain
	if !dm.HasPoolDomain(56) {
		t.Error("HasPoolDomain(56) should be true")
	}
	if dm.HasPoolDomain(1) {
		t.Error("HasPoolDomain(1) should be false")
	}

	// Test GetPoolDomainSeparator
	separator, ok := dm.GetPoolDomainSeparator(56)
	if !ok {
		t.Error("GetPoolDomainSeparator should return true for configured chain")
	}
	if len(separator) != 32 {
		t.Errorf("DomainSeparator length = %d, want 32", len(separator))
	}

	_, ok = dm.GetPoolDomainSeparator(1)
	if ok {
		t.Error("GetPoolDomainSeparator should return false for unconfigured chain")
	}

	// Test ChainIDs
	ids := dm.ChainIDs()
	if len(ids) != 1 || ids[0] != 56 {
		t.Errorf("ChainIDs = %v, want [56]", ids)
	}
}

func TestDomainManager_AddPoolDomainWithConfig(t *testing.T) {
	dm := NewDomainManager()

	// Use custom configuration
	dm.AddPoolDomainWithConfig(8453, "Custom Pool", "2", "0x2F46232bC664356BB38AA556Fe1aC939B2Cc7c74")

	domain := dm.GetPoolDomain(8453)
	if domain == nil {
		t.Fatal("GetPoolDomain returned nil")
	}
	if domain.Name != "Custom Pool" {
		t.Errorf("Domain.Name = %s, want Custom Pool", domain.Name)
	}
	if domain.Version != "2" {
		t.Errorf("Domain.Version = %s, want 2", domain.Version)
	}

	// Test empty values use defaults
	dm.AddPoolDomainWithConfig(1, "", "", "0x1234567890123456789012345678901234567890")
	domain = dm.GetPoolDomain(1)
	if domain.Name != DefaultDomainName {
		t.Errorf("Domain.Name = %s, want %s", domain.Name, DefaultDomainName)
	}
	if domain.Version != DefaultDomainVersion {
		t.Errorf("Domain.Version = %s, want %s", domain.Version, DefaultDomainVersion)
	}
}

func TestNewSignerFromHex(t *testing.T) {
	dm := NewDomainManager()
	dm.AddPoolDomain(56, common.HexToAddress("0x28D3a265f6d40867986004029ee91F4C9532fCC5"))

	// Valid private key
	validKey := "0x0000000000000000000000000000000000000000000000000000000000000001"
	signer, err := NewSignerFromHex(validKey, dm)
	if err != nil {
		t.Fatalf("NewSignerFromHex failed: %v", err)
	}
	if signer == nil {
		t.Fatal("Signer should not be nil")
	}

	// Verify address
	addr := signer.GetAddress()
	if addr == (common.Address{}) {
		t.Error("GetAddress returned zero address")
	}

	// Invalid private key
	invalidKey := "invalid"
	_, err = NewSignerFromHex(invalidKey, dm)
	if err == nil {
		t.Error("NewSignerFromHex should fail with invalid key")
	}
}

func TestSigner_SignMMQuote(t *testing.T) {
	dm := NewDomainManager()
	dm.AddPoolDomain(56, common.HexToAddress("0x28D3a265f6d40867986004029ee91F4C9532fCC5"))

	signer, err := NewSignerFromHex("0x0000000000000000000000000000000000000000000000000000000000000001", dm)
	if err != nil {
		t.Fatalf("NewSignerFromHex failed: %v", err)
	}

	amountOut, _ := new(big.Int).SetString("600000000000000000000", 10) // 600e18
	quote := &MMQuote{
		Pool:        common.HexToAddress("0x28D3a265f6d40867986004029ee91F4C9532fCC5"),
		From:        common.HexToAddress("0x1234567890123456789012345678901234567890"),
		To:          common.HexToAddress("0x1234567890123456789012345678901234567890"),
		InputToken:  common.HexToAddress("0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c"),
		OutputToken: common.HexToAddress("0x55d398326f99059fF775485246999027B3197955"),
		AmountIn:    big.NewInt(1000000000000000000), // 1e18
		AmountOut:   amountOut,
		Deadline:    big.NewInt(1735084800),
		Nonce:       big.NewInt(1),
		ExtraData:   []byte{},
	}

	sig, err := signer.SignMMQuote(56, quote)
	if err != nil {
		t.Fatalf("SignMMQuote failed: %v", err)
	}

	// Verify signature length (65 bytes: r(32) + s(32) + v(1))
	if len(sig) != 65 {
		t.Errorf("Signature length = %d, want 65", len(sig))
	}

	// Verify v value is 27 or 28
	v := sig[64]
	if v != 27 && v != 28 {
		t.Errorf("Signature v = %d, want 27 or 28", v)
	}

	// Ensure signature is deterministic
	sig2, _ := signer.SignMMQuote(56, quote)
	if string(sig) != string(sig2) {
		t.Error("Signature should be deterministic")
	}
}

func TestSigner_SignMMQuote_ChainNotConfigured(t *testing.T) {
	dm := NewDomainManager()
	// Don't add any domain

	signer, _ := NewSignerFromHex("0x0000000000000000000000000000000000000000000000000000000000000001", dm)

	amountOut2, _ := new(big.Int).SetString("600000000000000000000", 10) // 600e18
	quote := &MMQuote{
		Pool:        common.HexToAddress("0x28D3a265f6d40867986004029ee91F4C9532fCC5"),
		From:        common.HexToAddress("0x1234567890123456789012345678901234567890"),
		To:          common.HexToAddress("0x1234567890123456789012345678901234567890"),
		InputToken:  common.HexToAddress("0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c"),
		OutputToken: common.HexToAddress("0x55d398326f99059fF775485246999027B3197955"),
		AmountIn:    big.NewInt(1000000000000000000),
		AmountOut:   amountOut2,
		Deadline:    big.NewInt(1735084800),
		Nonce:       big.NewInt(1),
		ExtraData:   []byte{},
	}

	_, err := signer.SignMMQuote(56, quote)
	if err == nil {
		t.Error("SignMMQuote should fail when chain is not configured")
	}
}

func TestMinMaxSqrtPriceX96(t *testing.T) {
	// zeroForOne = true returns minimum price
	minPrice := MinMaxSqrtPriceX96(true)
	expectedMin := new(big.Int).Add(big.NewInt(4295128739), big.NewInt(1))
	if minPrice.Cmp(expectedMin) != 0 {
		t.Errorf("MinSqrtPriceX96 = %s, want %s", minPrice.String(), expectedMin.String())
	}

	// zeroForOne = false returns maximum price
	maxPrice := MinMaxSqrtPriceX96(false)
	maxStr := "1461446703485210103287273052203988822378723970342"
	expectedMax, _ := new(big.Int).SetString(maxStr, 10)
	expectedMax.Sub(expectedMax, big.NewInt(1))
	if maxPrice.Cmp(expectedMax) != 0 {
		t.Errorf("MaxSqrtPriceX96 = %s, want %s", maxPrice.String(), expectedMax.String())
	}
}

func TestDetermineZeroForOne(t *testing.T) {
	token0 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	token1 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	// sellerToken == token0 -> zeroForOne = true
	if !DetermineZeroForOne(token0, token0, token1) {
		t.Error("DetermineZeroForOne should return true when sellerToken == token0")
	}

	// sellerToken == token1 -> zeroForOne = false
	if DetermineZeroForOne(token1, token0, token1) {
		t.Error("DetermineZeroForOne should return false when sellerToken == token1")
	}
}

func TestEncodeExtraData(t *testing.T) {
	params := &ExtraDataParams{
		Pool:              common.HexToAddress("0x172fcD41E0913e95784454622d1c3724f546f849"),
		ZeroForOne:        true,
		SqrtPriceLimitX96: MinMaxSqrtPriceX96(true),
		CallbackData:      []byte{},
	}

	data, err := EncodeExtraData(params)
	if err != nil {
		t.Fatalf("EncodeExtraData failed: %v", err)
	}

	// Verify data is not empty
	if len(data) == 0 {
		t.Error("EncodeExtraData returned empty data")
	}

	// Test nil SqrtPriceLimitX96 uses default value
	params2 := &ExtraDataParams{
		Pool:         common.HexToAddress("0x172fcD41E0913e95784454622d1c3724f546f849"),
		ZeroForOne:   false,
		CallbackData: []byte{},
	}

	data2, err := EncodeExtraData(params2)
	if err != nil {
		t.Fatalf("EncodeExtraData with nil sqrtPriceLimit failed: %v", err)
	}
	if len(data2) == 0 {
		t.Error("EncodeExtraData returned empty data")
	}
}

func TestBuildCallbackData(t *testing.T) {
	payToken := common.HexToAddress("0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c")

	data, err := BuildCallbackData(payToken)
	if err != nil {
		t.Fatalf("BuildCallbackData failed: %v", err)
	}

	// ABI encoded address should be 32 bytes
	if len(data) != 32 {
		t.Errorf("BuildCallbackData length = %d, want 32", len(data))
	}
}

func TestHashExtraData(t *testing.T) {
	extraData := []byte{0x01, 0x02, 0x03}
	hash := HashExtraData(extraData)

	// Verify hash is 32 bytes
	if len(hash) != 32 {
		t.Errorf("HashExtraData length = %d, want 32", len(hash))
	}

	// Ensure it's deterministic
	hash2 := HashExtraData(extraData)
	if hash != hash2 {
		t.Error("HashExtraData should be deterministic")
	}
}

func TestNewSignerFromConfig(t *testing.T) {
	dm := NewDomainManager()
	dm.AddPoolDomain(56, common.HexToAddress("0x28D3a265f6d40867986004029ee91F4C9532fCC5"))

	// Test direct private key configuration
	cfg := &SignerConfig{
		PrivateKey: "0x0000000000000000000000000000000000000000000000000000000000000001",
	}
	signer, err := NewSignerFromConfig(cfg, dm)
	if err != nil {
		t.Fatalf("NewSignerFromConfig failed: %v", err)
	}
	if signer == nil {
		t.Fatal("Signer should not be nil")
	}

	// Test empty configuration
	emptyCfg := &SignerConfig{}
	_, err = NewSignerFromConfig(emptyCfg, dm)
	if err == nil {
		t.Error("NewSignerFromConfig should fail with empty config")
	}
}
