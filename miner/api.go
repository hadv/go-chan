package miner

import "github.com/hadv/go-chan/core/types"

// API exposes ethash related methods for the RPC interface.
type API struct {
	wrk *Worker
}

// SubmitWork can be used by external miner to submit their POW solution.
// It returns an indication if the work was accepted.
// Note either an invalid solution, a stale work a non-existent work will return false.
func (api *API) SubmitWork(nonce types.BlockNonce) bool {
	var errc = make(chan error, 1)
	select {
	case api.wrk.workCh <- &task{
		nonce: nonce,
		errc:  errc,
	}:
	case <-api.wrk.exitCh:
		return false
	}

	err := <-errc
	return err == nil
}
