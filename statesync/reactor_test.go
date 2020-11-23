package statesync

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/p2p"
	ssproto "github.com/tendermint/tendermint/proto/tendermint/statesync"
	proxymocks "github.com/tendermint/tendermint/proxy/mocks"
)

func TestReactor_Receive_ChunkRequest(t *testing.T) {
	testcases := map[string]struct {
		request        *ssproto.ChunkRequest
		chunk          []byte
		expectResponse *ssproto.ChunkResponse
	}{
		"chunk is returned": {
			&ssproto.ChunkRequest{Height: 1, Format: 1, Index: 1},
			[]byte{1, 2, 3},
			&ssproto.ChunkResponse{Height: 1, Format: 1, Index: 1, Chunk: []byte{1, 2, 3}}},
		"empty chunk is returned, as nil": {
			&ssproto.ChunkRequest{Height: 1, Format: 1, Index: 1},
			[]byte{},
			&ssproto.ChunkResponse{Height: 1, Format: 1, Index: 1, Chunk: nil}},
		"nil (missing) chunk is returned as missing": {
			&ssproto.ChunkRequest{Height: 1, Format: 1, Index: 1},
			nil,
			&ssproto.ChunkResponse{Height: 1, Format: 1, Index: 1, Missing: true},
		},
	}

	for name, tc := range testcases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			// Mock ABCI connection to return local snapshots
			conn := &proxymocks.AppConnSnapshot{}
			conn.On("LoadSnapshotChunkSync", abci.RequestLoadSnapshotChunk{
				Height: tc.request.Height,
				Format: tc.request.Format,
				Chunk:  tc.request.Index,
			}).Return(&abci.ResponseLoadSnapshotChunk{Chunk: tc.chunk}, nil)

			// mock peer to store response, if found
			peer, _ := simplePeer(t, "FF")

			var response *ssproto.ChunkResponse
			received := true

			if tc.expectResponse != nil {
				received = false
				peer.On("Send", ChunkChannel, mock.Anything).Run(func(args mock.Arguments) {
					msg, err := decodeMsg(args[1].([]byte))
					require.NoError(t, err)
					response = msg.(*ssproto.ChunkResponse)
					received = true
				}).Return(true)
			}

			// Start a reactor and send a ssproto.ChunkRequest, then wait for and check
			// response.
			shim := p2p.NewShim("StateSync", GetChannelShims())

			cfg := config.DefaultP2PConfig()
			p2pSwitch := p2p.MakeSwitch(cfg, 1, "testing", "123.123.123", func(_ int, sw *p2p.Switch) *p2p.Switch {
				p2p.AddPeerToSwitchPeerSet(sw, peer)
				sw.AddReactor(shim.Name, shim)
				return sw
			})

			r := NewReactor(
				shim.Logger,
				p2pSwitch,
				conn,
				nil,
				shim.GetChannel(p2p.ChannelID(SnapshotChannel)),
				shim.GetChannel(p2p.ChannelID(ChunkChannel)),
				shim.PeerUpdateCh,
				"",
			)

			ctx, cancel := context.WithCancel(context.Background())
			go r.Run(ctx)
			require.NoError(t, shim.Start())

			t.Cleanup(func() {
				cancel()

				if err := shim.Stop(); err != nil {
					t.Error(err)
				}
			})

			shim.Receive(ChunkChannel, peer, mustEncodeMsg(tc.request))
			tryUntil(t, func() bool { return received }, time.Millisecond, time.Second)
			assert.Equal(t, tc.expectResponse, response)

			conn.AssertExpectations(t)
			peer.AssertExpectations(t)
		})
	}
}

func TestReactor_Receive_SnapshotsRequest(t *testing.T) {
	testcases := map[string]struct {
		snapshots       []*abci.Snapshot
		expectResponses []*ssproto.SnapshotsResponse
	}{
		"no snapshots": {nil, []*ssproto.SnapshotsResponse{}},
		">10 unordered snapshots": {
			[]*abci.Snapshot{
				{Height: 1, Format: 2, Chunks: 7, Hash: []byte{1, 2}, Metadata: []byte{1}},
				{Height: 2, Format: 2, Chunks: 7, Hash: []byte{2, 2}, Metadata: []byte{2}},
				{Height: 3, Format: 2, Chunks: 7, Hash: []byte{3, 2}, Metadata: []byte{3}},
				{Height: 1, Format: 1, Chunks: 7, Hash: []byte{1, 1}, Metadata: []byte{4}},
				{Height: 2, Format: 1, Chunks: 7, Hash: []byte{2, 1}, Metadata: []byte{5}},
				{Height: 3, Format: 1, Chunks: 7, Hash: []byte{3, 1}, Metadata: []byte{6}},
				{Height: 1, Format: 4, Chunks: 7, Hash: []byte{1, 4}, Metadata: []byte{7}},
				{Height: 2, Format: 4, Chunks: 7, Hash: []byte{2, 4}, Metadata: []byte{8}},
				{Height: 3, Format: 4, Chunks: 7, Hash: []byte{3, 4}, Metadata: []byte{9}},
				{Height: 1, Format: 3, Chunks: 7, Hash: []byte{1, 3}, Metadata: []byte{10}},
				{Height: 2, Format: 3, Chunks: 7, Hash: []byte{2, 3}, Metadata: []byte{11}},
				{Height: 3, Format: 3, Chunks: 7, Hash: []byte{3, 3}, Metadata: []byte{12}},
			},
			[]*ssproto.SnapshotsResponse{
				{Height: 3, Format: 4, Chunks: 7, Hash: []byte{3, 4}, Metadata: []byte{9}},
				{Height: 3, Format: 3, Chunks: 7, Hash: []byte{3, 3}, Metadata: []byte{12}},
				{Height: 3, Format: 2, Chunks: 7, Hash: []byte{3, 2}, Metadata: []byte{3}},
				{Height: 3, Format: 1, Chunks: 7, Hash: []byte{3, 1}, Metadata: []byte{6}},
				{Height: 2, Format: 4, Chunks: 7, Hash: []byte{2, 4}, Metadata: []byte{8}},
				{Height: 2, Format: 3, Chunks: 7, Hash: []byte{2, 3}, Metadata: []byte{11}},
				{Height: 2, Format: 2, Chunks: 7, Hash: []byte{2, 2}, Metadata: []byte{2}},
				{Height: 2, Format: 1, Chunks: 7, Hash: []byte{2, 1}, Metadata: []byte{5}},
				{Height: 1, Format: 4, Chunks: 7, Hash: []byte{1, 4}, Metadata: []byte{7}},
				{Height: 1, Format: 3, Chunks: 7, Hash: []byte{1, 3}, Metadata: []byte{10}},
			},
		},
	}

	for name, tc := range testcases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			// mock ABCI connection to return local snapshots
			conn := &proxymocks.AppConnSnapshot{}
			conn.On("ListSnapshotsSync", abci.RequestListSnapshots{}).Return(&abci.ResponseListSnapshots{
				Snapshots: tc.snapshots,
			}, nil)

			// mock peer to catch responses and store them in a slice
			responses := []*ssproto.SnapshotsResponse{}
			received := true

			peer, _ := simplePeer(t, "FF")

			if len(tc.expectResponses) > 0 {
				received = false
				peer.On("Send", SnapshotChannel, mock.Anything).Run(func(args mock.Arguments) {
					msg, err := decodeMsg(args[1].([]byte))
					require.NoError(t, err)
					responses = append(responses, msg.(*ssproto.SnapshotsResponse))
					received = true
				}).Return(true)
			}

			// Start a reactor and send a ssproto.ChunkRequest, then wait for and check
			// response.
			shim := p2p.NewShim("StateSync", GetChannelShims())

			cfg := config.DefaultP2PConfig()
			p2pSwitch := p2p.MakeSwitch(cfg, 1, "testing", "123.123.123", func(_ int, sw *p2p.Switch) *p2p.Switch {
				p2p.AddPeerToSwitchPeerSet(sw, peer)
				sw.AddReactor(shim.Name, shim)
				return sw
			})

			r := NewReactor(
				shim.Logger,
				p2pSwitch,
				conn,
				nil,
				shim.GetChannel(p2p.ChannelID(SnapshotChannel)),
				shim.GetChannel(p2p.ChannelID(ChunkChannel)),
				shim.PeerUpdateCh,
				"",
			)

			ctx, cancel := context.WithCancel(context.Background())
			go r.Run(ctx)
			require.NoError(t, shim.Start())

			t.Cleanup(func() {
				cancel()

				if err := shim.Stop(); err != nil {
					t.Error(err)
				}
			})

			shim.Receive(SnapshotChannel, peer, mustEncodeMsg(&ssproto.SnapshotsRequest{}))
			tryUntil(t, func() bool { return received }, time.Millisecond, time.Second)
			assert.Equal(t, tc.expectResponses, responses)

			conn.AssertExpectations(t)
			peer.AssertExpectations(t)
		})
	}
}
