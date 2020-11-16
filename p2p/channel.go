package p2p

import (
	"google.golang.org/protobuf/proto"
)

// ChannelID is an arbitrary channel ID.
type ChannelID uint16

// Envelope specifies the message receiver and sender.
type Envelope struct {
	From      PeerID        // Message sender, or empty for outbound messages.
	To        PeerID        // Message receiver, or empty for inbound messages.
	Broadcast bool          // Send message to all connected peers, ignoring To.
	Message   proto.Message // Payload.
}

// Channel is a bidirectional channel for Protobuf message exchange with peers.
type Channel struct {
	// ID contains the channel ID.
	ID ChannelID

	// messageType specifies the type of messages exchanged via the channel, and
	// is used e.g. for automatic unmarshaling.
	messageType proto.Message

	// In is a channel for receiving inbound messages. Envelope.From is always
	// set.
	In <-chan Envelope

	// Out is a channel for sending outbound messages. Envelope.To or Broadcast
	// must be set, otherwise the message is discarded.
	Out chan<- Envelope

	// Error is a channel for reporting peer errors to the router, typically used
	// when peers send an invalid or malignant message.
	Error chan<- PeerError
}

// Close closes the channel, and is equivalent to close(Channel.Out). This will
// cause Channel.In to be closed when appropriate. The ID can then be reused.
func (c *Channel) Close() error { return nil }
