package signer

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	RFQVaultDomainName    = "RFQ MMVault"
	RFQVaultDomainVersion = "1"
)

type EIP712Domain struct {
	Name              string
	Version           string
	ChainID           *big.Int
	VerifyingContract common.Address
}

func (d *EIP712Domain) DomainSeparator() []byte {
	typeHash := crypto.Keccak256Hash([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"))
	nameHash := crypto.Keccak256Hash([]byte(d.Name))
	versionHash := crypto.Keccak256Hash([]byte(d.Version))

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

type DomainManager struct {
	vaultDomains map[uint64]*EIP712Domain
}

func NewDomainManager() *DomainManager {
	return &DomainManager{
		vaultDomains: make(map[uint64]*EIP712Domain),
	}
}

func (m *DomainManager) AddVaultDomain(chainID uint64, vaultAddr common.Address) {
	m.vaultDomains[chainID] = &EIP712Domain{
		Name:              RFQVaultDomainName,
		Version:           RFQVaultDomainVersion,
		ChainID:           big.NewInt(int64(chainID)),
		VerifyingContract: vaultAddr,
	}
}

func (m *DomainManager) GetVaultDomain(chainID uint64) *EIP712Domain {
	return m.vaultDomains[chainID]
}

func (m *DomainManager) GetVaultDomainSeparator(chainID uint64) ([]byte, bool) {
	domain := m.vaultDomains[chainID]
	if domain == nil {
		return nil, false
	}
	return domain.DomainSeparator(), true
}

func (m *DomainManager) HasVaultDomain(chainID uint64) bool {
	_, ok := m.vaultDomains[chainID]
	return ok
}

func (m *DomainManager) ChainIDs() []uint64 {
	ids := make([]uint64, 0, len(m.vaultDomains))
	for id := range m.vaultDomains {
		ids = append(ids, id)
	}
	return ids
}
