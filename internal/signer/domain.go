package signer

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// DarkPool RFQ Manager Domain default values
const (
	DefaultDomainName    = "RFQ Manager"
	DefaultDomainVersion = "1"
)

// EIP712Domain represents the EIP-712 Domain structure
type EIP712Domain struct {
	Name              string         // Domain name
	Version           string         // Domain version
	ChainID           *big.Int       // Chain ID
	VerifyingContract common.Address // Verifying contract address
}

// DomainSeparator calculates the EIP-712 Domain Separator
// Reference: https://eips.ethereum.org/EIPS/eip-712
func (d *EIP712Domain) DomainSeparator() []byte {
	// EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)
	typeHash := crypto.Keccak256Hash([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"))
	nameHash := crypto.Keccak256Hash([]byte(d.Name))
	versionHash := crypto.Keccak256Hash([]byte(d.Version))

	// ABI encode parameters
	bytes32Ty, _ := abi.NewType("bytes32", "", nil)
	uint256Ty, _ := abi.NewType("uint256", "", nil)
	addressTy, _ := abi.NewType("address", "", nil)

	args := abi.Arguments{
		{Type: bytes32Ty},
		{Type: bytes32Ty},
		{Type: bytes32Ty},
		{Type: uint256Ty},
		{Type: addressTy},
	}

	encoded, _ := args.Pack(typeHash, nameHash, versionHash, d.ChainID, d.VerifyingContract)
	return crypto.Keccak256(encoded)
}

// DomainManager manages multi-chain DarkPool RFQ Manager EIP-712 Domains
type DomainManager struct {
	rfqManagerDomains map[uint64]*EIP712Domain // chainId -> DarkPool RFQ Manager domain
}

// NewDomainManager creates a Domain manager
func NewDomainManager() *DomainManager {
	return &DomainManager{
		rfqManagerDomains: make(map[uint64]*EIP712Domain),
	}
}

// AddPoolDomain adds a DarkPool RFQ Manager Domain configuration
func (m *DomainManager) AddPoolDomain(chainID uint64, poolAddr common.Address) {
	m.rfqManagerDomains[chainID] = &EIP712Domain{
		Name:              DefaultDomainName,
		Version:           DefaultDomainVersion,
		ChainID:           big.NewInt(int64(chainID)),
		VerifyingContract: poolAddr,
	}
}

// AddPoolDomainWithConfig adds a DarkPool RFQ Manager Domain with full configuration
// Supports custom name and version (uses defaults if empty)
func (m *DomainManager) AddPoolDomainWithConfig(chainID uint64, name, version, poolAddr string) {
	if name == "" {
		name = DefaultDomainName
	}
	if version == "" {
		version = DefaultDomainVersion
	}
	m.rfqManagerDomains[chainID] = &EIP712Domain{
		Name:              name,
		Version:           version,
		ChainID:           big.NewInt(int64(chainID)),
		VerifyingContract: common.HexToAddress(poolAddr),
	}
}

// GetPoolDomain gets the DarkPool RFQ Manager Domain for a specified chain
func (m *DomainManager) GetPoolDomain(chainID uint64) *EIP712Domain {
	return m.rfqManagerDomains[chainID]
}

// GetPoolDomainSeparator gets the DarkPool RFQ Manager Domain Separator for a specified chain
func (m *DomainManager) GetPoolDomainSeparator(chainID uint64) ([]byte, bool) {
	domain := m.rfqManagerDomains[chainID]
	if domain == nil {
		return nil, false
	}
	return domain.DomainSeparator(), true
}

// HasRFQManagerDomain checks if a DarkPool RFQ Manager Domain is configured for a specified chain
func (m *DomainManager) HasRFQManagerDomain(chainID uint64) bool {
	_, ok := m.rfqManagerDomains[chainID]
	return ok
}

// ChainIDs returns all configured chain IDs
func (m *DomainManager) ChainIDs() []uint64 {
	ids := make([]uint64, 0, len(m.rfqManagerDomains))
	for id := range m.rfqManagerDomains {
		ids = append(ids, id)
	}
	return ids
}
