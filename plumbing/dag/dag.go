package dag

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-path"
	"github.com/ipfs/go-path/resolver"
	"github.com/ipfs/go-unixfs"
)

// DAG is a service for accessing the merkledag
type DAG struct {
	dserv ipld.DAGService
}

// NewDAG creates a DAG with a given DAGService
func NewDAG(dserv ipld.DAGService) *DAG {
	return &DAG{
		dserv: dserv,
	}
}

// GetNode returns the associated DAG node for the passed in CID.
func (dag *DAG) GetNode(ctx context.Context, ref string) (interface{}, error) {
	parsedRef, err := path.ParsePath(ref)
	if err != nil {
		return nil, err
	}

	resolver := resolver.NewBasicResolver(dag.dserv)

	objc, rem, err := resolver.ResolveToLastNode(ctx, parsedRef)
	if err != nil {
		return nil, err
	}

	obj, err := dag.dserv.Get(ctx, objc)
	if err != nil {
		return nil, err
	}

	var out interface{} = obj
	if len(rem) > 0 {
		final, _, err := obj.Resolve(rem)
		if err != nil {
			return nil, err
		}
		out = final
	}

	return out, nil
}

// GetFileSize returns the file size for a given Cid
func (dag *DAG) GetFileSize(ctx context.Context, c cid.Cid) (uint64, error) {
	fnode, err := dag.dserv.Get(ctx, c)
	if err != nil {
		return 0, err
	}
	switch n := fnode.(type) {
	case *merkledag.ProtoNode:
		return unixfs.DataSize(n.Data())
	case *merkledag.RawNode:
		return n.Size()
	default:
		return 0, fmt.Errorf("unrecognized node type: %T", fnode)
	}
}
