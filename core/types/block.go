package types

import (
	"encoding/binary"
	"math/big"
	"unsafe"

	"github.com/hadv/go-chan/common"
	"github.com/hadv/go-chan/common/hexutil"
	"github.com/hadv/go-chan/crypto/sha3"
	"github.com/hadv/go-chan/rlp"
)

// A BlockNonce is a 64-bit hash which proves (combined with the
// mix-hash) that a sufficient amount of computation has been carried
// out on a block.
type BlockNonce [8]byte

// EncodeNonce converts the given integer to a block nonce.
func EncodeNonce(i uint64) BlockNonce {
	var n BlockNonce
	binary.BigEndian.PutUint64(n[:], i)
	return n
}

// Uint64 returns the integer value of a block nonce.
func (n BlockNonce) Uint64() uint64 {
	return binary.BigEndian.Uint64(n[:])
}

// MarshalText encodes n as a hex string with 0x prefix.
func (n BlockNonce) MarshalText() ([]byte, error) {
	return hexutil.Bytes(n[:]).MarshalText()
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (n *BlockNonce) UnmarshalText(input []byte) error {
	return hexutil.UnmarshalFixedText("BlockNonce", input, n[:])
}

// Block
type Block struct {
	Difficulty *big.Int   `json:"difficulty"       gencodec:"required"`
	Number     *big.Int   `json:"number"           gencodec:"required"`
	Time       *big.Int   `json:"timestamp"        gencodec:"required"`
	Nonce      BlockNonce `json:"nonce"            gencodec:"required"`
}

// field type overrides for gencodec
type blockMarshaling struct {
	Difficulty *hexutil.Big
	Number     *hexutil.Big
	Time       *hexutil.Big
	Hash       common.Hash `json:"hash"` // adds call to Hash() in MarshalJSON
}

// Hash returns the block hash of the header, which is simply the keccak256 hash of its
// RLP encoding.
func (b *Block) Hash() common.Hash {
	return rlpHash(b)
}

// Size returns the approximate memory used by all internal contents. It is used
// to approximate and limit the memory consumption of various caches.
func (b *Block) Size() common.StorageSize {
	return common.StorageSize(unsafe.Sizeof(*b)) + common.StorageSize((b.Difficulty.BitLen()+b.Number.BitLen()+b.Time.BitLen())/8)
}

func rlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}

// CopyBlock creates a deep copy of a block to prevent side effects from
// modifying a header variable.
func CopyBlock(b *Block) *Block {
	cpy := *b
	if cpy.Time = new(big.Int); b.Time != nil {
		cpy.Time.Set(b.Time)
	}
	if cpy.Difficulty = new(big.Int); b.Difficulty != nil {
		cpy.Difficulty.Set(b.Difficulty)
	}
	if cpy.Number = new(big.Int); b.Number != nil {
		cpy.Number.Set(b.Number)
	}
	return &cpy
}
