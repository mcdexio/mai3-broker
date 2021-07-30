package message

import (
	"fmt"
)

const WsTypeOrderChange = "orderChange"

type WebSocketMessage struct {
	ChannelID string      `json:"channel_id"`
	Payload   interface{} `json:"payload"`
}

type WebSocketOrderChangePayload struct {
	Type            string      `json:"type"`
	Order           interface{} `json:"order"`
	BlockNumber     uint64      `json:"blockNumber"`
	TransactionHash string      `json:"transaction_hash"`
}

const AccountChannelPrefix = "TraderAddress"

func GetAccountChannelID(address string) string {
	return fmt.Sprintf("%s#%s", AccountChannelPrefix, address)
}
