package commands_test

import (
	"context"
	"io/ioutil"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"gx/ipfs/QmPVkJMTeRC6iBByPWdrRkD3BE5UXsj5HPzb4kPqL186mS/testify/assert"
	"gx/ipfs/QmPVkJMTeRC6iBByPWdrRkD3BE5UXsj5HPzb4kPqL186mS/testify/require"
	"gx/ipfs/QmQmhotPUzVrMEWNK3x1R5jQ5ZHWyL7tVUrmRPjrBrvyCb/go-ipfs-files"
	cbor "gx/ipfs/QmcZLyosDwMKdB6NLRsiss9HXzDPhVhhRtPy67JFKTDQDX/go-ipld-cbor"
	"gx/ipfs/QmekxXDhCxCJRNuzmHreuaT3BsuJcsjcXWNrtV9C8DRHtd/go-multibase"

	"github.com/filecoin-project/go-filecoin/actor/builtin/paymentbroker"
	"github.com/filecoin-project/go-filecoin/address"
	th "github.com/filecoin-project/go-filecoin/testhelpers"
	"github.com/filecoin-project/go-filecoin/tools/fast"
	"github.com/filecoin-project/go-filecoin/tools/fast/series"
	localplugin "github.com/filecoin-project/go-filecoin/tools/iptb-plugins/filecoin/local"
	"github.com/filecoin-project/go-filecoin/types"
)

func init() {
	_, _ = os.Getwd()

	series.GlobalSleepDelay = time.Second * 5
}

func VoucherFromString(data string) (paymentbroker.PaymentVoucher, error) {
	_, cborVoucher, err := multibase.Decode(data)
	if err != nil {
		return paymentbroker.PaymentVoucher{}, err
	}

	var voucher paymentbroker.PaymentVoucher
	if err = cbor.DecodeInto(cborVoucher, &voucher); err != nil {
		return paymentbroker.PaymentVoucher{}, err
	}

	return voucher, nil
}

// BasiFastSetup creates a environment and a single node, and environment options
func BasicFastSetup(t *testing.T, ctx context.Context, fastenvOpts fast.EnvironmentOpts) (fast.Environment, *fast.Filecoin, func() *fast.Filecoin) {
	require := require.New(t)

	// Create a directory for the test using the test name (mostly for FAST)
	// TODO(tperson) use a different TempDir that uses MkdirAll
	dir, err := ioutil.TempDir("", strings.Replace(t.Name(), "/", ".", -1))
	require.NoError(err)

	// Create an environment that includes a genesis block with 1MM FIL
	env, err := fast.NewEnvironmentMemoryGenesis(big.NewInt(1000000), dir)

	// Setup options for nodes.
	options := make(map[string]string)
	options[localplugin.AttrLogJSON] = "1"                               // Enable JSON logs
	options[localplugin.AttrLogLevel] = "5"                              // Set log level to Debug
	options[localplugin.AttrUseSmallSectors] = "true"                    // Enable small sectors
	options[localplugin.AttrFilecoinBinary] = th.MustGetFilecoinBinary() // Get the filecoin binary

	genesisURI := env.GenesisCar()
	genesisMiner, err := env.GenesisMiner()
	require.NoError(err)

	fastenvOpts = fast.EnvironmentOpts{
		InitOpts:   append([]fast.ProcessInitOption{fast.POGenesisFile(genesisURI)}, fastenvOpts.InitOpts...),
		DaemonOpts: append([]fast.ProcessDaemonOption{fast.POBlockTime(series.GlobalSleepDelay)}, fastenvOpts.DaemonOpts...),
	}

	// Create a node for the test
	genesis, err := env.NewProcess(ctx, localplugin.PluginName, options, fastenvOpts)
	require.NoError(err)

	err = series.SetupGenesisNode(ctx, genesis, genesisMiner.Address, files.NewReaderFile(genesisMiner.Owner))
	require.NoError(err)

	NewNode := func() *fast.Filecoin {
		p, err := env.NewProcess(ctx, localplugin.PluginName, options, fastenvOpts)
		require.NoError(err)

		err = series.InitAndStart(ctx, p)
		require.NoError(err)

		err = series.Connect(ctx, genesis, p)
		require.NoError(err)

		err = series.SendFilecoinDefaults(ctx, genesis, p, 10000)
		require.NoError(err)

		return p
	}

	return env, genesis, NewNode
}

func TestPaymentChannelCreateSuccess(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	// This test should run in 20 block times
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(20*series.GlobalSleepDelay))
	defer cancel()

	// Get basic testing environment
	env, _, NewNode := BasicFastSetup(t, ctx, fast.EnvironmentOpts{})

	// Teardown after test ends
	defer env.Teardown(ctx)

	// Start test
	targetDaemon := NewNode()
	payerDaemon := NewNode()

	addrs, err := targetDaemon.AddressLs(ctx)
	require.NoError(err)
	targetAddr := addrs[0]

	addrs, err = payerDaemon.AddressLs(ctx)
	require.NoError(err)
	payerAddr := addrs[0]

	channelExpiry := types.NewBlockHeight(20)
	channelAmount := types.NewAttoFILFromFIL(1000)

	WithPaymentChannel(t, ctx, payerDaemon, payerAddr, targetAddr, channelAmount, channelExpiry, func(chanid *types.ChannelID) {
		require.NotNil(chanid)
	})
}

// WithPaymentChannel takes a filecoin daemon and constructs a payment channel to the `target` for the `amt` for `eol`, and calls fn with the channel id
func WithPaymentChannel(t *testing.T, ctx context.Context, p *fast.Filecoin, from, target address.Address, amt *types.AttoFIL, eol *types.BlockHeight, fn func(chanid *types.ChannelID)) {
	require := require.New(t)

	mcid, err := p.PaychCreate(ctx, target, amt, eol, fast.AOFromAddr(from), fast.AOPrice(big.NewFloat(0)), fast.AOLimit(300))
	require.NoError(err)

	response, err := p.MessageWait(ctx, mcid)
	require.NoError(err)

	chanid := types.NewChannelIDFromBytes(response.Receipt.Return[0])
	fn(chanid)
}

func TestPaymentChannelLs(t *testing.T) {
	t.Parallel()

	t.Run("Works with default payer", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)
		assert := assert.New(t)

		// This test should run in 20 block times
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(20*series.GlobalSleepDelay))
		defer cancel()

		// Get basic testing environment
		env, _, NewNode := BasicFastSetup(t, ctx, fast.EnvironmentOpts{})

		// Teardown after test ends
		defer env.Teardown(ctx)

		// Start test
		targetDaemon := NewNode()
		payerDaemon := NewNode()

		addrs, err := targetDaemon.AddressLs(ctx)
		require.NoError(err)
		targetAddr := addrs[0]

		addrs, err = payerDaemon.AddressLs(ctx)
		require.NoError(err)
		payerAddr := addrs[0]

		channelExpiry := types.NewBlockHeight(20)
		channelAmount := types.NewAttoFILFromFIL(1000)

		WithPaymentChannel(t, ctx, payerDaemon, payerAddr, targetAddr, channelAmount, channelExpiry, func(chanid *types.ChannelID) {
			channels, err := payerDaemon.PaychLs(ctx)
			require.NoError(err)

			assert.Len(channels, 1)

			channel := channels[chanid.String()]
			assert.Equal(channelAmount, channel.Amount)
			assert.Equal(channelExpiry, channel.Eol)
			assert.Equal(targetAddr, channel.Target)
			assert.Equal(types.ZeroAttoFIL, channel.AmountRedeemed)
		})
	})

	t.Run("Works with specified payer", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)
		assert := assert.New(t)

		// This test should run in 20 block times
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(20*series.GlobalSleepDelay))
		defer cancel()

		// Get basic testing environment
		env, _, NewNode := BasicFastSetup(t, ctx, fast.EnvironmentOpts{})

		// Teardown after test ends
		defer env.Teardown(ctx)

		// Start test
		targetDaemon := NewNode()
		payerDaemon := NewNode()

		addrs, err := targetDaemon.AddressLs(ctx)
		require.NoError(err)
		targetAddr := addrs[0]

		addrs, err = payerDaemon.AddressLs(ctx)
		require.NoError(err)
		payerAddr := addrs[0]

		channelExpiry := types.NewBlockHeight(20)
		channelAmount := types.NewAttoFILFromFIL(1000)

		WithPaymentChannel(t, ctx, payerDaemon, payerAddr, targetAddr, channelAmount, channelExpiry, func(chanid *types.ChannelID) {
			channels, err := payerDaemon.PaychLs(ctx, fast.AOPayer(payerAddr))
			require.NoError(err)

			assert.Len(channels, 1)

			channel := channels[chanid.String()]
			assert.Equal(channelAmount, channel.Amount)
			assert.Equal(channelExpiry, channel.Eol)
			assert.Equal(targetAddr, channel.Target)
			assert.Equal(types.ZeroAttoFIL, channel.AmountRedeemed)
		})
	})

	t.Run("No results when listing with different from address", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)
		assert := assert.New(t)

		// This test should run in 20 block times
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(20*series.GlobalSleepDelay))
		defer cancel()

		// Get basic testing environment
		env, _, NewNode := BasicFastSetup(t, ctx, fast.EnvironmentOpts{})

		// Teardown after test ends
		defer env.Teardown(ctx)

		// Start test
		targetDaemon := NewNode()
		payerDaemon := NewNode()

		addrs, err := targetDaemon.AddressLs(ctx)
		require.NoError(err)
		targetAddr := addrs[0]

		addrs, err = payerDaemon.AddressLs(ctx)
		require.NoError(err)
		payerAddr := addrs[0]

		channelExpiry := types.NewBlockHeight(20)
		channelAmount := types.NewAttoFILFromFIL(1000)

		WithPaymentChannel(t, ctx, payerDaemon, payerAddr, targetAddr, channelAmount, channelExpiry, func(chanid *types.ChannelID) {
			channels, err := payerDaemon.PaychLs(ctx, fast.AOFromAddr(targetAddr))
			require.NoError(err)

			assert.Len(channels, 0)
		})
	})
}

func TestPaymentChannelVoucherSuccess(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	assert := assert.New(t)

	// This test should run in 20 block times
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(20*series.GlobalSleepDelay))
	defer cancel()

	// Get basic testing environment
	env, _, NewNode := BasicFastSetup(t, ctx, fast.EnvironmentOpts{})

	// Teardown after test ends
	defer env.Teardown(ctx)

	// Start test
	targetDaemon := NewNode()
	payerDaemon := NewNode()

	addrs, err := targetDaemon.AddressLs(ctx)
	require.NoError(err)
	targetAddr := addrs[0]

	addrs, err = payerDaemon.AddressLs(ctx)
	require.NoError(err)
	payerAddr := addrs[0]

	channelExpiry := types.NewBlockHeight(20)
	channelAmount := types.NewAttoFILFromFIL(1000)

	WithPaymentChannel(t, ctx, payerDaemon, payerAddr, targetAddr, channelAmount, channelExpiry, func(chanid *types.ChannelID) {
		voucherAmount := types.NewAttoFILFromFIL(10)
		voucherValidAt := types.NewBlockHeight(0)
		voucherStr, err := payerDaemon.PaychVoucher(ctx, chanid, voucherAmount, fast.AOFromAddr(payerAddr), fast.AOValidAt(voucherValidAt))
		require.NoError(err)

		voucher, err := VoucherFromString(voucherStr)
		require.NoError(err)

		assert.Equal(voucherAmount, &voucher.Amount)
	})
}

func TestPaymentChannelRedeemSuccess(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	assert := assert.New(t)

	// This test should run in 20 block times
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(20*series.GlobalSleepDelay))
	defer cancel()

	// Get basic testing environment
	env, _, NewNode := BasicFastSetup(t, ctx, fast.EnvironmentOpts{})

	// Teardown after test ends
	defer env.Teardown(ctx)

	// Start test
	targetDaemon := NewNode()
	payerDaemon := NewNode()

	addrs, err := targetDaemon.AddressLs(ctx)
	require.NoError(err)
	targetAddr := addrs[0]

	addrs, err = payerDaemon.AddressLs(ctx)
	require.NoError(err)
	payerAddr := addrs[0]

	channelExpiry := types.NewBlockHeight(20)
	channelAmount := types.NewAttoFILFromFIL(1000)

	WithPaymentChannel(t, ctx, payerDaemon, payerAddr, targetAddr, channelAmount, channelExpiry, func(chanid *types.ChannelID) {
		voucherAmount := types.NewAttoFILFromFIL(10)
		voucherValidAt := types.NewBlockHeight(0)
		voucherStr, err := payerDaemon.PaychVoucher(ctx, chanid, voucherAmount, fast.AOFromAddr(payerAddr), fast.AOValidAt(voucherValidAt))
		require.NoError(err)

		// TODO(tperson) allow setting Price / Limit on process?
		mcid, err := targetDaemon.PaychRedeem(ctx, voucherStr, fast.AOFromAddr(targetAddr), fast.AOPrice(big.NewFloat(0)), fast.AOLimit(300))
		require.NoError(err)

		_, err = targetDaemon.MessageWait(ctx, mcid)
		require.NoError(err)

		channels, err := targetDaemon.PaychLs(ctx, fast.AOFromAddr(payerAddr))
		require.NoError(err)

		channel := channels[chanid.String()]
		assert.Equal(channelAmount, channel.Amount)
		assert.Equal(voucherAmount, channel.AmountRedeemed)
	})
}

func TestPaymentChannelRedeemTooEarlyFails(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	assert := assert.New(t)

	// This test should run in 20 block times
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(20*series.GlobalSleepDelay))
	defer cancel()

	// Get basic testing environment
	env, _, NewNode := BasicFastSetup(t, ctx, fast.EnvironmentOpts{})

	// Teardown after test ends
	defer env.Teardown(ctx)

	// Start test
	targetDaemon := NewNode()
	payerDaemon := NewNode()

	addrs, err := targetDaemon.AddressLs(ctx)
	require.NoError(err)
	targetAddr := addrs[0]

	addrs, err = payerDaemon.AddressLs(ctx)
	require.NoError(err)
	payerAddr := addrs[0]

	channelExpiry := types.NewBlockHeight(20)
	channelAmount := types.NewAttoFILFromFIL(1000)

	WithPaymentChannel(t, ctx, payerDaemon, payerAddr, targetAddr, channelAmount, channelExpiry, func(chanid *types.ChannelID) {
		voucherAmount := types.NewAttoFILFromFIL(10)
		voucherValidAt := types.NewBlockHeight(10)
		voucherStr, err := payerDaemon.PaychVoucher(ctx, chanid, voucherAmount, fast.AOFromAddr(payerAddr), fast.AOValidAt(voucherValidAt))
		require.NoError(err)

		// TODO(tperson) allow setting Price / Limit on process?
		mcid, err := targetDaemon.PaychRedeem(ctx, voucherStr, fast.AOFromAddr(targetAddr), fast.AOPrice(big.NewFloat(0)), fast.AOLimit(300))
		require.NoError(err)

		_, err = targetDaemon.MessageWait(ctx, mcid)
		require.NoError(err)

		channels, err := targetDaemon.PaychLs(ctx, fast.AOFromAddr(payerAddr))
		require.NoError(err)

		channel := channels[chanid.String()]
		assert.Equal(channelAmount, channel.Amount)
		assert.Equal(types.ZeroAttoFIL, channel.AmountRedeemed)
	})
}

func TestPaymentChannelReclaimSuccess(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	assert := assert.New(t)

	// This test should run in 20 block times
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(40*series.GlobalSleepDelay))
	defer cancel()

	// Get basic testing environment
	env, _, NewNode := BasicFastSetup(t, ctx, fast.EnvironmentOpts{})

	// Teardown after test ends
	defer env.Teardown(ctx)

	// Start test
	targetDaemon := NewNode()
	payerDaemon := NewNode()

	addrs, err := targetDaemon.AddressLs(ctx)
	require.NoError(err)
	targetAddr := addrs[0]

	addrs, err = payerDaemon.AddressLs(ctx)
	require.NoError(err)
	payerAddr := addrs[0]

	channelExpiry := types.NewBlockHeight(5)
	channelAmount := types.NewAttoFILFromFIL(1000)

	balanceBefore, err := payerDaemon.WalletBalance(ctx, payerAddr)
	require.NoError(err)

	WithPaymentChannel(t, ctx, payerDaemon, payerAddr, targetAddr, channelAmount, channelExpiry, func(chanid *types.ChannelID) {
		voucherAmount := types.NewAttoFILFromFIL(10)
		voucherValidAt := types.NewBlockHeight(0)
		voucherStr, err := payerDaemon.PaychVoucher(ctx, chanid, voucherAmount, fast.AOFromAddr(payerAddr), fast.AOValidAt(voucherValidAt))
		require.NoError(err)

		// TODO(tperson) allow setting Price / Limit on process?
		mcid, err := targetDaemon.PaychRedeem(ctx, voucherStr, fast.AOFromAddr(targetAddr), fast.AOPrice(big.NewFloat(0)), fast.AOLimit(300))
		require.NoError(err)

		_, err = targetDaemon.MessageWait(ctx, mcid)
		require.NoError(err)

		channels, err := targetDaemon.PaychLs(ctx, fast.AOFromAddr(payerAddr))
		require.NoError(err)

		channel := channels[chanid.String()]
		assert.Equal(channelAmount, channel.Amount)
		assert.Equal(voucherAmount, channel.AmountRedeemed)

		series.WaitForBlockHeight(ctx, payerDaemon, channel.Eol)

		mcid, err = payerDaemon.PaychReclaim(ctx, chanid, fast.AOFromAddr(payerAddr), fast.AOPrice(big.NewFloat(0)), fast.AOLimit(300))
		require.NoError(err)

		_, err = payerDaemon.MessageWait(ctx, mcid)
		require.NoError(err)

		channels, err = payerDaemon.PaychLs(ctx, fast.AOFromAddr(payerAddr))
		require.NoError(err)
		require.Len(channels, 0)

		balanceAfter, err := payerDaemon.WalletBalance(ctx, payerAddr)

		assert.Equal(balanceBefore, balanceAfter.Add(voucherAmount))
	})
}

func TestPaymentChannelCloseSuccess(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	assert := assert.New(t)

	// This test should run in 20 block times
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(40*series.GlobalSleepDelay))
	defer cancel()

	// Get basic testing environment
	env, _, NewNode := BasicFastSetup(t, ctx, fast.EnvironmentOpts{})

	// Teardown after test ends
	defer env.Teardown(ctx)

	// Start test
	targetDaemon := NewNode()
	payerDaemon := NewNode()

	addrs, err := targetDaemon.AddressLs(ctx)
	require.NoError(err)
	targetAddr := addrs[0]

	channelExpiry := types.NewBlockHeight(5)
	channelAmount := types.NewAttoFILFromFIL(1000)

	addrs, err = payerDaemon.AddressLs(ctx)
	require.NoError(err)
	payerAddr := addrs[0]

	payerBalanceBefore, err := payerDaemon.WalletBalance(ctx, payerAddr)
	require.NoError(err)

	targetBalanceBefore, err := targetDaemon.WalletBalance(ctx, targetAddr)
	require.NoError(err)

	WithPaymentChannel(t, ctx, payerDaemon, payerAddr, targetAddr, channelAmount, channelExpiry, func(chanid *types.ChannelID) {
		voucherAmount := types.NewAttoFILFromFIL(10)
		voucherValidAt := types.NewBlockHeight(0)
		voucherStr, err := payerDaemon.PaychVoucher(ctx, chanid, voucherAmount, fast.AOFromAddr(payerAddr), fast.AOValidAt(voucherValidAt))
		require.NoError(err)

		// TODO(tperson) allow setting Price / Limit on process?
		mcid, err := targetDaemon.PaychClose(ctx, voucherStr, fast.AOFromAddr(targetAddr), fast.AOPrice(big.NewFloat(0)), fast.AOLimit(300))
		require.NoError(err)

		_, err = targetDaemon.MessageWait(ctx, mcid)
		require.NoError(err)

		channels, err := targetDaemon.PaychLs(ctx, fast.AOFromAddr(payerAddr))
		require.NoError(err)
		require.Len(channels, 0)

		payerBalanceAfter, err := payerDaemon.WalletBalance(ctx, payerAddr)
		assert.Equal(payerBalanceBefore.Sub(voucherAmount), payerBalanceAfter)

		targetBalanceAfter, err := targetDaemon.WalletBalance(ctx, targetAddr)
		assert.Equal(targetBalanceBefore.Add(voucherAmount), targetBalanceAfter)
	})
}

func TestPaymentChannelExtendSuccess(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	assert := assert.New(t)

	// This test should run in 20 block times
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(20*series.GlobalSleepDelay))
	defer cancel()

	// Get basic testing environment
	env, _, NewNode := BasicFastSetup(t, ctx, fast.EnvironmentOpts{})

	// Teardown after test ends
	defer env.Teardown(ctx)

	// Start test
	targetDaemon := NewNode()
	payerDaemon := NewNode()

	addrs, err := targetDaemon.AddressLs(ctx)
	require.NoError(err)
	targetAddr := addrs[0]

	channelExpiry := types.NewBlockHeight(5)
	channelAmount := types.NewAttoFILFromFIL(1000)

	addrs, err = payerDaemon.AddressLs(ctx)
	require.NoError(err)
	payerAddr := addrs[0]

	WithPaymentChannel(t, ctx, payerDaemon, payerAddr, targetAddr, channelAmount, channelExpiry, func(chanid *types.ChannelID) {
		channels, err := payerDaemon.PaychLs(ctx)
		require.NoError(err)

		assert.Len(channels, 1)

		channel := channels[chanid.String()]
		assert.Equal(channelAmount, channel.Amount)
		assert.Equal(channelExpiry, channel.Eol)
		assert.Equal(targetAddr, channel.Target)
		assert.Equal(types.ZeroAttoFIL, channel.AmountRedeemed)

		extendAmount := types.NewAttoFILFromFIL(100)
		extendExpiry := types.NewBlockHeight(100)

		mcid, err := payerDaemon.PaychExtend(ctx, chanid, extendAmount, extendExpiry, fast.AOFromAddr(payerAddr), fast.AOPrice(big.NewFloat(0)), fast.AOLimit(300))
		require.NoError(err)

		_, err = payerDaemon.MessageWait(ctx, mcid)
		require.NoError(err)

		channels, err = payerDaemon.PaychLs(ctx)
		require.NoError(err)

		assert.Len(channels, 1)

		channel = channels[chanid.String()]
		assert.Equal(channelAmount.Add(extendAmount), channel.Amount)
		assert.Equal(extendExpiry, channel.Eol)
		assert.Equal(targetAddr, channel.Target)
		assert.Equal(types.ZeroAttoFIL, channel.AmountRedeemed)
	})
}
