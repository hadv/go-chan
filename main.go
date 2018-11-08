package main

import (
	"fmt"
	"math/big"
	"time"

	"github.com/hadv/go-chan/core/types"
	"github.com/hadv/go-chan/log"
	"github.com/hadv/go-chan/miner"
)

const (
	// resultQueueSize is the size of channel listening to sealing result.
	resultQueueSize = 10
)

func main() {
	fmt.Println("Starting . . . ")
	var (
		stopCh   chan struct{}
		resultCh chan *types.Block
	)
	// interrupt aborts the in-flight sealing task.
	interrupt := func() {
		if stopCh != nil {
			close(stopCh)
			stopCh = nil
		}
	}
	number := big.NewInt(0)
	miner := miner.New()
	go func() {
		for {
			interrupt()
			stopCh = make(chan struct{})
			resultCh = make(chan *types.Block)
			if err := miner.Seal(&types.Block{
				Difficulty: big.NewInt(1000000),
				Time:       big.NewInt(time.Now().Unix()),
				Number:     number,
			}, resultCh, stopCh); err != nil {
				log.Warn("Block sealing failed", "err", err)
			}

			select {
			case result := <-resultCh:
				fmt.Println("🔨 mined potential block,", "number =", result.Number.Uint64(), "time =", result.Time.String())
			}

			number.Add(number, big.NewInt(1))
		}
	}()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	api := miner.API()
	for {
		select {
		case <-ticker.C:
			if ok := api.SubmitWork(types.EncodeNonce(1)); ok {
				fmt.Println("API: submit work from remote is correct...")
			}
		}
	}
}
