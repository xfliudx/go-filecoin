package series

import (
	"context"

	"github.com/filecoin-project/go-filecoin/tools/fast"
	"github.com/filecoin-project/go-filecoin/types"
)

// WaitForBlockHeight will inspect the chain head and wait till the height is equal to or
// greater than the provide height `bh`
func WaitForBlockHeight(ctx context.Context, client *fast.Filecoin, bh *types.BlockHeight) error {
	for {
		// Client waits around for the deal to be sealed
		tipset, err := client.ChainHead(ctx)
		if err != nil {
			return err
		}

		if types.NewBlockHeight(uint64(tipset[0].Height)).GreaterEqual(bh) {
			break
		}

		SleepDelay()
	}

	return nil
}
