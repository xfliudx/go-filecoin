package chain_test

import (
	"context"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-filecoin/chain"
	"github.com/filecoin-project/go-filecoin/types"
)

func TestGetParentTipSet(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	require := require.New(t)
	ctx := context.Background()
	store := newBlockStore()

	root := store.NewBlock(0)
	b11 := store.NewBlock(1, root)
	b12 := store.NewBlock(2, root)
	b21 := store.NewBlock(3, b11, b12)

	t.Run("root has empty parent", func(t *testing.T) {
		ts := requireTipset(t, root)
		parent, e := chain.GetParentTipSet(ctx, store, ts)
		require.NoError(e)
		assert.Empty(parent)
	})
	t.Run("plural tipset", func(t *testing.T) {
		ts := requireTipset(t, b11, b12)
		parent, e := chain.GetParentTipSet(ctx, store, ts)
		require.NoError(e)
		assert.True(requireTipset(t, root).Equals(parent))
	})
	t.Run("plural parent", func(t *testing.T) {
		ts := requireTipset(t, b21)
		parent, e := chain.GetParentTipSet(ctx, store, ts)
		require.NoError(e)
		assert.True(requireTipset(t, b11, b12).Equals(parent))
	})
}

func TestIterAncestors(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	ctx := context.Background()
	store := newBlockStore()

	root := store.NewBlock(0)
	b11 := store.NewBlock(1, root)
	b12 := store.NewBlock(2, root)
	b21 := store.NewBlock(3, b11, b12)

	t0 := requireTipset(t, root)
	t1 := requireTipset(t, b11, b12)
	t2 := requireTipset(t, b21)

	it := chain.IterAncestors(ctx, store, t2)
	assert.False(it.Complete())
	assert.True(t2.Equals(it.Value()))

	assert.NoError(it.Next())
	assert.False(it.Complete())
	assert.True(t1.Equals(it.Value()))

	assert.NoError(it.Next())
	assert.False(it.Complete())
	assert.True(t0.Equals(it.Value()))

	assert.NoError(it.Next())
	assert.True(it.Complete())
}

type fakeBlockStore struct {
	blocks map[cid.Cid]*types.Block
	seq    int
}

func newBlockStore() *fakeBlockStore {
	return &fakeBlockStore{
		make(map[cid.Cid]*types.Block),
		0,
	}
}

func (bs *fakeBlockStore) GetBlock(ctx context.Context, cid cid.Cid) (*types.Block, error) {
	block, ok := bs.blocks[cid]
	if ok {
		return block, nil
	}
	return nil, errors.New("No such block")
}

func (bs *fakeBlockStore) NewBlock(nonce uint64, parents ...*types.Block) *types.Block {
	b := &types.Block{
		Nonce: types.Uint64(nonce),
	}

	if len(parents) > 0 {
		b.Height = parents[0].Height + 1
		b.StateRoot = parents[0].StateRoot
		for _, p := range parents {
			b.Parents.Add(p.Cid())
		}
	}

	bs.blocks[b.Cid()] = b
	return b
}

func requireTipset(t *testing.T, blocks ...*types.Block) types.TipSet {
	set, err := types.NewTipSet(blocks...)
	require.NoError(t, err)
	return set
}
