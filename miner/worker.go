package miner

import (
	crand "crypto/rand"
	"encoding/binary"
	"errors"
	"math"
	"math/big"
	"math/rand"
	"runtime"
	"sync"

	"github.com/hadv/go-chan/common"
	"github.com/hadv/go-chan/core/types"
	"github.com/hadv/go-chan/crypto"
	"github.com/hadv/go-chan/crypto/sha3"
	"github.com/hadv/go-chan/log"
	"github.com/hadv/go-chan/rlp"
)

const (
	mixBytes     = 128 // Width of mix
	hashBytes    = 64  // Hash length in bytes
	hashWords    = 16  // Number of 32 bit ints in a hash
	loopAccesses = 64  // Number of accesses in hashimoto loop
)

var (
	// two256 is a big integer representing 2^256
	two256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), big.NewInt(0))

	// errInvalidSealResult invalid or stale proof-of-work solution
	errInvalidSealResult = errors.New("invalid or stale proof-of-work solution")
)

type Worker struct {
	// Mining related fields
	rand    *rand.Rand    // Properly seeded random source for nonces
	threads int           // Number of threads to mine on if mining
	update  chan struct{} // Notification channel to update mining parameters

	// Remote
	workCh chan *task

	lock   sync.Mutex // Ensures thread safety for the in-memory caches and mining fields
	exitCh chan chan error
}

func New() *Worker {
	worker := &Worker{
		update: make(chan struct{}),
		workCh: make(chan *task),
	}
	go worker.remote()
	return worker
}

func (w *Worker) remote() {
	submitWork := func(nonce types.BlockNonce) bool {
		return true
	}

	for {
		select {
		case result := <-w.workCh:
			if submitWork(result.nonce) {
				result.errc <- nil
			} else {
				result.errc <- errInvalidSealResult
			}

		case errc := <-w.exitCh:
			errc <- nil
			return
		}
	}
}

// Seal implements consensus.Engine, attempting to find a nonce that satisfies
// the block's difficulty requirements.
func (w *Worker) Seal(block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
	// Create a runner and the multiple search threads it directs
	abort := make(chan struct{})

	w.lock.Lock()
	threads := w.threads
	if w.rand == nil {
		seed, err := crand.Int(crand.Reader, big.NewInt(math.MaxInt64))
		if err != nil {
			w.lock.Unlock()
			return err
		}
		w.rand = rand.New(rand.NewSource(seed.Int64()))
	}
	w.lock.Unlock()
	if threads == 0 {
		threads = runtime.NumCPU()
	}
	if threads < 0 {
		threads = 0 // Allows disabling local mining without extra logic around local/remote
	}
	var (
		pend   sync.WaitGroup
		locals = make(chan *types.Block)
	)
	for i := 0; i < threads; i++ {
		pend.Add(1)
		go func(id int, nonce uint64) {
			defer pend.Done()
			w.mine(block, nonce, abort, locals)
		}(i, uint64(w.rand.Int63()))
	}
	// Wait until sealing is terminated or a nonce is found
	go func() {
		var result *types.Block
		select {
		case <-stop:
			// Outside abort, stop all miner threads
			close(abort)
		case result = <-locals:
			// One of the threads found a block, abort all others
			select {
			case results <- result:
			default:
				log.Warn("Sealing result is not read by miner", "mode", "local", "sealhash", w.SealHash(block))
			}
			close(abort)
		case <-w.update:
			// Thread count was changed on user request, restart
			close(abort)
			if err := w.Seal(block, results, stop); err != nil {
				log.Error("Failed to restart sealing after update", "err", err)
			}
		}
		// Wait for all miners to terminate and return the block
		pend.Wait()
	}()
	return nil
}

// mine is the actual proof-of-work miner that searches for a nonce starting from
// seed that results in correct final block difficulty.
func (w *Worker) mine(block *types.Block, seed uint64, abort chan struct{}, found chan *types.Block) {
	// Extract some data from the block
	var (
		hash   = w.SealHash(block).Bytes()
		target = new(big.Int).Div(two256, block.Difficulty)
	)
	// Start generating random nonces until we abort or find a good one
	var (
		nonce = seed
	)
search:
	for {
		select {
		case <-abort:
			break search
		default:
			result := pow(hash, nonce)
			if new(big.Int).SetBytes(result).Cmp(target) <= 0 {
				// Correct nonce found, create a new header with it
				blk := types.CopyBlock(block)
				blk.Nonce = types.EncodeNonce(nonce)

				// Seal and return a block (if still needed)
				select {
				case found <- blk:
					log.Trace("Ethash nonce found and reported", "attempts", nonce-seed, "nonce", nonce)
				case <-abort:
					log.Trace("Ethash nonce found but discarded", "attempts", nonce-seed, "nonce", nonce)
				}
				break search
			}
			nonce++
		}
	}
}

// pow
func pow(hash []byte, nonce uint64) []byte {
	log.Info("hashimoto")
	// Combine header+nonce into a 64 byte seed
	seed := make([]byte, 40)
	copy(seed, hash)
	binary.LittleEndian.PutUint64(seed[32:], nonce)
	seed = crypto.Keccak512(seed)

	// Start the mix with replicated seed
	mix := make([]uint32, mixBytes/4)
	for i := 0; i < len(mix); i++ {
		mix[i] = binary.LittleEndian.Uint32(seed[i%16*4:])
	}
	// Compress mix
	for i := 0; i < len(mix); i += 4 {
		mix[i/4] = fnv(fnv(fnv(mix[i], mix[i+1]), mix[i+2]), mix[i+3])
	}
	mix = mix[:len(mix)/4]

	digest := make([]byte, common.HashLength)
	for i, val := range mix {
		binary.LittleEndian.PutUint32(digest[i*4:], val)
	}
	return crypto.Keccak256(append(seed, digest...))
}

// a non-associative substitute for XOR. Note that we multiply the prime with
// the full 32-bit input, in contrast with the FNV-1 spec which multiplies the
// prime with one byte (octet) in turn.
func fnv(a, b uint32) uint32 {
	return a*0x01000193 ^ b
}

// SealHash returns the hash of a block prior to it being sealed.
func (w *Worker) SealHash(block *types.Block) (hash common.Hash) {
	hasher := sha3.NewKeccak256()

	rlp.Encode(hasher, []interface{}{
		block.Difficulty,
		block.Number,
		block.Time,
	})
	hasher.Sum(hash[:0])
	return hash
}

// API
func (w *Worker) API() *API {
	return &API{w}
}

type task struct {
	errc  chan error
	nonce types.BlockNonce
}
