package msg

import (
	"context"
	"sync"

	"github.com/filecoin-project/go-filecoin/abi"
	"github.com/filecoin-project/go-filecoin/actor"
	"github.com/filecoin-project/go-filecoin/address"
	"github.com/filecoin-project/go-filecoin/chain"
	"github.com/filecoin-project/go-filecoin/consensus"
	"github.com/filecoin-project/go-filecoin/core"
	"github.com/filecoin-project/go-filecoin/state"
	"github.com/filecoin-project/go-filecoin/types"

	"gx/ipfs/QmR8BauakNcBa3RbE4nbQu76PDiJgoQgz8AJdhJuiU4TAw/go-cid"
	"gx/ipfs/QmVmDhyTTUcQXFD1rRQ64fGLMSAoaQvNH3hwuaCFAPq2hy/errors"
)

// Topic is the network pubsub topic identifier on which new messages are announced.
const Topic = "/fil/msgs"

// Abstracts over a store of blockchain state.
type chainState interface {
	// LatestState returns the latest chain state.
	LatestState(ctx context.Context) (state.Tree, error)
}

// PublishFunc is a function the Sender calls to publish a message to the network.
type PublishFunc func(topic string, data []byte) error

// Sender is plumbing implementation that knows how to send a message.
type Sender struct {
	// Signs messages.
	signer types.Signer
	// Provides actor state
	chainState chainState
	// Provides the current block height
	blockTimer core.BlockTimer
	// Tracks inbound messages for mining
	inbox *core.MessagePool
	// Tracks outbound messages
	outbox *core.MessageQueue
	// Validates messages before sending them.
	validator consensus.SignedMessageValidator
	// Invoked to publish the new message to the network.
	publish PublishFunc
	// Protects the "next nonce" calculation to avoid collisions.
	l sync.Mutex
}

// NewSender returns a new Sender. There should be exactly one of these per node because
// sending locks to reduce nonce collisions.
func NewSender(signer types.Signer, chainReader chain.ReadStore, blockTimer core.BlockTimer,
	msgQueue *core.MessageQueue, msgPool *core.MessagePool,
	validator consensus.SignedMessageValidator, publish PublishFunc) *Sender {
	return &Sender{
		signer:     signer,
		chainState: chainReader,
		blockTimer: blockTimer,
		inbox:      msgPool,
		outbox:     msgQueue,
		validator:  validator,
		publish:    publish,
	}
}

// Send sends a message. See api description.
func (s *Sender) Send(ctx context.Context, from, to address.Address, value *types.AttoFIL, gasPrice types.AttoFIL, gasLimit types.GasUnits, method string, params ...interface{}) (cid.Cid, error) {
	encodedParams, err := abi.ToEncodedValues(params...)
	if err != nil {
		return cid.Undef, errors.Wrap(err, "invalid params")
	}

	// Lock to avoid race for message nonce.
	s.l.Lock()
	defer s.l.Unlock()

	st, err := s.chainState.LatestState(ctx)
	if err != nil {
		return cid.Undef, errors.Wrap(err, "failed to load state from chain")
	}

	fromActor, err := st.GetActor(ctx, from)
	if err != nil {
		return cid.Undef, errors.Wrapf(err, "no actor at address %s", from)
	}

	nonce, err := nextNonce(fromActor, s.outbox, from)
	if err != nil {
		return cid.Undef, errors.Wrapf(err, "failed calculating nonce for actor %s", from)
	}

	msg := types.NewMessage(from, to, nonce, value, method, encodedParams)
	smsg, err := types.NewSignedMessage(*msg, s.signer, gasPrice, gasLimit)
	if err != nil {
		return cid.Undef, errors.Wrap(err, "failed to sign message")
	}

	err = s.validator.Validate(ctx, smsg, fromActor)
	if err != nil {
		return cid.Undef, errors.Wrap(err, "invalid message")
	}

	smsgdata, err := smsg.Marshal()
	if err != nil {
		return cid.Undef, errors.Wrap(err, "failed to marshal message")
	}

	height, err := s.blockTimer.BlockHeight()
	if err != nil {
		return cid.Undef, errors.Wrap(err, "failed to get block height")
	}

	// Add to the local message queue/pool at the last possible moment before broadcasting to network.
	if err := s.outbox.Enqueue(smsg, height); err != nil {
		return cid.Undef, errors.Wrap(err, "failed to add message to outbound queue")
	}
	if _, err := s.inbox.Add(smsg); err != nil {
		return cid.Undef, errors.Wrap(err, "failed to add message to message pool")
	}

	if err = s.publish(Topic, smsgdata); err != nil {
		return cid.Undef, errors.Wrap(err, "failed to publish message to network")
	}

	log.Debugf("MessageSend with message: %s", smsg)
	return smsg.Cid()
}

// nextNonce returns the next expected nonce value for an account actor. This is the larger
// of the actor's nonce value, or one greater than the largest nonce from the actor found in the message pool.
func nextNonce(act *actor.Actor, outbox *core.MessageQueue, address address.Address) (uint64, error) {
	actorNonce, err := actor.NextNonce(act)
	if err != nil {
		return 0, err
	}

	poolNonce, found := outbox.LargestNonce(address)
	if found && poolNonce >= actorNonce {
		return poolNonce + 1, nil
	}
	return actorNonce, nil
}
