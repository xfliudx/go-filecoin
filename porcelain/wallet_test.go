package porcelain_test

import (
	"context"
	"testing"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-filecoin/actor"
	"github.com/filecoin-project/go-filecoin/address"
	"github.com/filecoin-project/go-filecoin/porcelain"
	"github.com/filecoin-project/go-filecoin/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type walletTestPlumbing struct {
	balance *types.AttoFIL
}

func (wtp *walletTestPlumbing) ActorGet(ctx context.Context, addr address.Address) (*actor.Actor, error) {
	testActor := actor.NewActor(cid.Undef, wtp.balance)
	return testActor, nil
}

func TestWalletBalance(t *testing.T) {
	t.Run("Returns the correct value for wallet balance", func(t *testing.T) {
		assert := assert.New(t)
		require := require.New(t)
		ctx := context.Background()

		expectedBalance := types.NewAttoFILFromFIL(20)
		plumbing := &walletTestPlumbing{
			balance: expectedBalance,
		}
		balance, err := porcelain.WalletBalance(ctx, plumbing, address.Undef)
		require.NoError(err)

		assert.Equal(expectedBalance, balance)
	})
}
