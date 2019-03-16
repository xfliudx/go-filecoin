package impl

import (
	"context"
	"io"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-peer"

	"github.com/filecoin-project/go-filecoin/address"
	"github.com/filecoin-project/go-filecoin/types"
)

type nodeRetrievalClient struct {
	api      *nodeAPI
	plumbing retrievalClientPorcelainAPI
}

type retrievalClientPorcelainAPI interface {
	MinerGetPeerID(ctx context.Context, minerAddr address.Address) (peer.ID, error)
	types.Signer
}

func newNodeRetrievalClient(api *nodeAPI, plumbing retrievalClientPorcelainAPI) *nodeRetrievalClient {
	return &nodeRetrievalClient{
		api:      api,
		plumbing: plumbing,
	}
}

func (nrc *nodeRetrievalClient) RetrievePiece(ctx context.Context, pieceCID cid.Cid, minerAddr address.Address) (io.ReadCloser, error) {
	minerPeerID, err := nrc.plumbing.MinerGetPeerID(ctx, minerAddr)
	if err != nil {
		return nil, err
	}

	return nrc.api.node.RetrievalClient.RetrievePiece(ctx, minerPeerID, pieceCID)
}
