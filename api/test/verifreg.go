package test

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/node/impl"
	verifreg4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/verifreg"

	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/big"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	logging "github.com/ipfs/go-log/v2"
)

func init() {
	logging.SetAllLoggers(logging.LevelInfo)
	err := os.Setenv("BELLMAN_NO_GPU", "1")
	if err != nil {
		panic(fmt.Sprintf("failed to set BELLMAN_NO_GPU env variable: %s", err))
	}
	build.InsecurePoStValidation = true
}

func AddVerifiedClient(t *testing.T, b APIBuilderWithRKH) {

	rkhKey, err := wallet.GenerateKey(types.KTSecp256k1)
	if err != nil {
		return
	}

	nodes, miners := b(t, []FullNodeOpts{FullNodeWithLatestActorsAt(-1)}, OneMiner, *rkhKey)
	api := nodes[0].FullNode.(*impl.FullNodeAPI)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//Add verifier
	verifier, err := api.WalletDefaultAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.WalletImport(ctx, &rkhKey.KeyInfo); err != nil {
		t.Fatal(err)
	}
	params, err := actors.SerializeParams(&verifreg4.AddVerifierParams{Address: verifier, Allowance: big.NewInt(100000000000)})
	if err != nil {
		t.Fatal(err)
	}
	msg := &types.Message{
		To:     verifreg.Address,
		From:   rkhKey.Address,
		Method: verifreg.Methods.AddVerifier,
		Params: params,
		Value:  big.Zero(),
	}

	bm := NewBlockMiner(ctx, t, miners[0], 100*time.Millisecond)
	bm.MineBlocks()
	defer bm.Stop()

	sm, err := api.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		t.Fatal("AddVerifier failed: ", err)
	}
	res, err := api.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Receipt.ExitCode != 0 {
		t.Fatal("did not successfully send message")
	}

	//Assign datacap to a client
	datacap := big.NewInt(10000)
	clientAddress, err := api.WalletNew(ctx, types.KTBLS)
	if err != nil {
		t.Fatal(err)
	}

	params, err = actors.SerializeParams(&verifreg4.AddVerifiedClientParams{Address: clientAddress, Allowance: datacap})
	if err != nil {
		t.Fatal(err)
	}

	msg = &types.Message{
		To:     verifreg.Address,
		From:   verifier,
		Method: verifreg.Methods.AddVerifiedClient,
		Params: params,
		Value:  big.Zero(),
	}

	sm, err = api.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		t.Fatal("AddVerifiedClient faield: ", err)
	}
	res, err = api.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Receipt.ExitCode != 0 {
		t.Fatal("did not successfully send message")
	}

	//check datacap balance
	dcap, err := api.StateVerifiedClientStatus(ctx, clientAddress, types.EmptyTSK)
	if err != nil {
		t.Fatal(err)
	}
	if !dcap.Equals(datacap) {
		t.Fatal("")
	}

	//try to assign datacap to the same client should fail for actor v4 and below
	params, err = actors.SerializeParams(&verifreg4.AddVerifiedClientParams{Address: clientAddress, Allowance: datacap})
	if err != nil {
		t.Fatal(err)
	}

	msg = &types.Message{
		To:     verifreg.Address,
		From:   verifier,
		Method: verifreg.Methods.AddVerifiedClient,
		Params: params,
		Value:  big.Zero(),
	}

	if _, err = api.MpoolPushMessage(ctx, msg, nil); !strings.Contains(err.Error(), "verified client already exists") {
		t.Fatal("Add datacap to an exist verified client should fail")
	}
}
