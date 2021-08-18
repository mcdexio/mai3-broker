package websocket

import (
	"fmt"
	"github.com/mcdexio/mai3-broker/common/message"
	logger "github.com/sirupsen/logrus"
	"strings"
	"sync"
)

// Channel is a basic type implemented IChannel
type Channel struct {
	ID      string
	Clients sync.Map

	Subscribe   chan *Client
	Unsubscribe chan string
	Messages    chan *message.WebSocketMessage
}

func (c *Channel) GetID() string {
	return c.ID
}

func (c *Channel) AddSubscriber(client *Client) {

	c.Subscribe <- client
}

func (c *Channel) RemoveSubscriber(ID string) {
	c.Unsubscribe <- ID
}

func (c *Channel) AddMessage(msg *message.WebSocketMessage) {
	c.Messages <- msg
}

func (c *Channel) UnsubscribeChan() chan string {
	return c.Unsubscribe
}

func (c *Channel) SubScribeChan() chan *Client {
	return c.Subscribe
}

func (c *Channel) MessagesChan() chan *message.WebSocketMessage {
	return c.Messages
}

func (c *Channel) handleMessage(msg *message.WebSocketMessage) {
	c.Clients.Range(func(k, v interface{}) bool {
		client := v.(*Client)
		client.SendMsg(msg.Payload)
		logger.Debugf("send message to client: channel: %s, payload: %s", msg.ChannelID, msg.Payload)
		return true
	})
}

func (c *Channel) handleSubscriber(client *Client) {
	c.Clients.Store(client.ID, client)

	logger.Debugf("client(%s) joins channel(%s)", client.ID, c.ID)
}

func (c *Channel) handleUnsubscriber(ID string) {
	c.Clients.Delete(ID)

	logger.Debugf("client(%s) leaves channel(%s)", ID, c.ID)
}

func runChannel(c IChannel) {
	for {
		select {
		case msg := <-c.MessagesChan():
			c.handleMessage(msg)
		case client := <-c.SubScribeChan():
			c.handleSubscriber(client)
		case ID := <-c.UnsubscribeChan():
			c.handleUnsubscriber(ID)
		}
	}
}

var allChannels = make(map[string]IChannel, 10)
var allChannelsMutex = &sync.RWMutex{}

func findChannel(id string) IChannel {
	allChannelsMutex.RLock()
	defer allChannelsMutex.RUnlock()

	return allChannels[id]
}

func saveChannel(channel IChannel) {
	allChannelsMutex.Lock()
	defer allChannelsMutex.Unlock()

	if _, ok := allChannels[channel.GetID()]; !ok {
		allChannels[channel.GetID()] = channel
	}
}

func RemoveSubscriberAllChannels(client *Client) {
	allChannelsMutex.RLock()
	defer allChannelsMutex.RUnlock()
	for _, channel := range allChannels {
		channel.RemoveSubscriber(client.ID)
	}
}

func createChannelByID(channelID string) (IChannel, error) {
	parts := strings.Split(channelID, "#")
	prefix := parts[0]

	var channel IChannel

	if creatorFunc := channelCreators[prefix]; creatorFunc != nil {
		var err error
		channel, err = creatorFunc(channelID)
		if err != nil {
			return nil, fmt.Errorf("create channel by id fail:%w", err)
		}

	} else {
		channel = createBaseChannel(channelID)
	}

	saveChannel(channel)
	go runChannel(channel)

	return channel, nil
}

func createBaseChannel(channelID string) *Channel {
	return &Channel{
		ID:          channelID,
		Subscribe:   make(chan *Client),
		Unsubscribe: make(chan string),
		Messages:    make(chan *message.WebSocketMessage),
	}
}
