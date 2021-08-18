package orderbook

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/cevaris/ordered_map"
	"github.com/mcdexio/mai3-broker/common/mai3/utils"
	"github.com/mcdexio/mai3-broker/common/model"
	"github.com/petar/GoLLRB/llrb"
	"github.com/shopspring/decimal"
)

type (
	MemoryOrder struct {
		ID                   string          `json:"id"`
		LiquidityPoolAddress string          `json:"liquidityPoolAddress"`
		PerpetualIndex       int64           `json:"perpetualIndex"`
		Price                decimal.Decimal `json:"price"`
		TriggerPrice         decimal.Decimal `json:"triggerPrice"`
		Amount               decimal.Decimal `json:"amount"`
		MinTradeAmount       decimal.Decimal `json:"minTradeAmount"`
		Type                 model.OrderType `json:"type"`
		Trader               string          `json:"trader"`
	}
)

type priceLevel struct {
	price       decimal.Decimal
	totalAmount decimal.Decimal
	orderMap    *ordered_map.OrderedMap
}

func newPriceLevel(price decimal.Decimal) *priceLevel {
	return &priceLevel{
		price:       price,
		totalAmount: decimal.Zero,
		orderMap:    ordered_map.NewOrderedMap(),
	}
}

func (p *priceLevel) Len() int {
	return p.orderMap.Len()
}

func (p *priceLevel) InsertOrder(order *MemoryOrder) error {
	if _, ok := p.orderMap.Get(order.ID); ok {
		return fmt.Errorf("priceLevel can't add order which is already in this priceLevel. priceLevel: %s, orderID: %s", p.price.String(), order.ID)
	}

	p.orderMap.Set(order.ID, order)
	p.totalAmount = p.totalAmount.Add(order.Amount)
	return nil
}

func (p *priceLevel) RemoveOrder(orderID string) (*MemoryOrder, error) {
	orderItem, ok := p.orderMap.Get(orderID)

	if !ok {
		return nil, fmt.Errorf("priceLevel can't remove order which is not in this priceLevel. priceLevel: %s:%w", p.price.String(), OrderNotFoundError)
	}

	order := orderItem.(*MemoryOrder)
	p.orderMap.Delete(order.ID)
	p.totalAmount = p.totalAmount.Sub(order.Amount)
	return order, nil
}

func (p *priceLevel) GetOrder(id string) (order *MemoryOrder, exist bool) {
	orderItem, exist := p.orderMap.Get(id)
	if !exist {
		return nil, exist
	}

	return orderItem.(*MemoryOrder), exist
}

func (p *priceLevel) GetOrders() (orders []*MemoryOrder) {
	iter := p.orderMap.IterFunc()
	for kv, ok := iter(); ok; kv, ok = iter() {
		orders = append(orders, kv.Value.(*MemoryOrder))
	}
	return
}

func (p *priceLevel) ChangeOrder(orderID string, changeAmount decimal.Decimal) error {
	orderItem, ok := p.orderMap.Get(orderID)

	if !ok {
		return fmt.Errorf("can't change order which is not in this priceLevel. priceLevel: %s:%w", p.price.String(), OrderNotFoundError)
	}

	order := orderItem.(*MemoryOrder)
	oldAmount := order.Amount
	newAmount := order.Amount.Add(changeAmount)
	if !newAmount.IsZero() && !utils.HasTheSameSign(newAmount, oldAmount) {
		return fmt.Errorf("can't change order[%s], after change the amount is negative. old amount=%s change amount=%s",
			order.ID, order.Amount, changeAmount)
	}

	order.Amount = newAmount
	if newAmount.IsZero() {
		p.orderMap.Delete(orderID)
	}

	p.totalAmount = p.totalAmount.Add(changeAmount)
	return nil
}

func (p *priceLevel) Less(item llrb.Item) bool {
	another := item.(*priceLevel)
	return p.price.LessThan(another.price)
}

// Orderbook ...
type Orderbook struct {
	bidsTree  *llrb.LLRB
	asksTree  *llrb.LLRB
	bidPrices map[string]interface{}
	askPrices map[string]interface{}

	lock sync.RWMutex
}

// NewOrderbook return a new book
func NewOrderbook() *Orderbook {
	book := &Orderbook{
		bidsTree:  llrb.New(),
		asksTree:  llrb.New(),
		bidPrices: make(map[string]interface{}),
		askPrices: make(map[string]interface{}),
	}

	return book
}

func (book *Orderbook) InsertOrder(order *MemoryOrder) error {
	//startTime := time.Now().UTC()
	book.lock.Lock()
	defer book.lock.Unlock()

	//log.Debug("cost in lock, InsertOrder :", order.ID, float64(time.Since(startTime))/1000000)

	var tree *llrb.LLRB
	if order.Amount.IsNegative() {
		tree = book.asksTree
	} else {
		tree = book.bidsTree
	}

	price := tree.Get(newPriceLevel(order.Price))

	if price == nil {
		price = newPriceLevel(order.Price)
		tree.InsertNoReplace(price)
	}

	err := price.(*priceLevel).InsertOrder(order)
	if err != nil {
		return fmt.Errorf("InsertOrder:%w", err)
	}

	if order.Amount.IsNegative() {
		book.askPrices[order.Price.String()] = nil
	} else {
		book.bidPrices[order.Price.String()] = nil
	}

	return nil
}

var OrderNotFoundError = errors.New("order not found")

func (book *Orderbook) RemoveOrder(order *MemoryOrder) error {
	book.lock.Lock()
	defer book.lock.Unlock()

	var tree *llrb.LLRB
	isBuy := true
	if order.Amount.IsNegative() {
		tree = book.asksTree
		isBuy = false
	} else {
		tree = book.bidsTree
	}

	plItem := tree.Get(newPriceLevel(order.Price))
	if plItem == nil {
		return fmt.Errorf("remove order: find price level fail, price=%s:%w", order.Price, OrderNotFoundError)
	}

	price := plItem.(*priceLevel)

	if price == nil {
		return fmt.Errorf("price is nil when RemoveOrder, order: %+v", order)
	}

	_, err := price.RemoveOrder(order.ID)
	if err != nil {
		return fmt.Errorf("remove order fom price level fail:%w", err)
	}

	if price.Len() <= 0 {
		tree.Delete(price)
		if !isBuy {
			delete(book.askPrices, order.Price.String())
		} else {
			delete(book.bidPrices, order.Price.String())
		}
	}

	return nil
}

func (book *Orderbook) ChangeOrder(order *MemoryOrder, changeAmount decimal.Decimal) error {
	book.lock.Lock()
	defer book.lock.Unlock()

	var tree *llrb.LLRB
	isBuy := true
	if order.Amount.IsNegative() {
		tree = book.asksTree
		isBuy = false
	} else {
		tree = book.bidsTree
	}

	plItem := tree.Get(newPriceLevel(order.Price))

	if plItem == nil {
		return fmt.Errorf("can't change order which is not in this orderbook. order: %+v:%w", order, OrderNotFoundError)
	}

	price := plItem.(*priceLevel)
	if price == nil {
		return fmt.Errorf("pl is nil when ChangeOrder, order: %+v", order)
	}

	if err := price.ChangeOrder(order.ID, changeAmount); err != nil {
		return fmt.Errorf("change order fail:%w", err)
	}
	if price.Len() <= 0 {
		tree.Delete(price)
		if !isBuy {
			delete(book.askPrices, order.Price.String())
		} else {
			delete(book.bidPrices, order.Price.String())
		}
	}

	return nil
}

func (book *Orderbook) GetOrder(id string, isAsk bool, price decimal.Decimal) (*MemoryOrder, bool) {
	book.lock.Lock()
	defer book.lock.Unlock()

	var tree *llrb.LLRB
	if isAsk {
		tree = book.asksTree
	} else {
		tree = book.bidsTree
	}

	pl := tree.Get(newPriceLevel(price))

	if pl == nil {
		return nil, false
	}

	return pl.(*priceLevel).GetOrder(id)
}

// MaxBid ...
func (book *Orderbook) MaxBid() *decimal.Decimal {
	book.lock.Lock()
	defer book.lock.Unlock()

	maxItem := book.bidsTree.Max()
	if maxItem != nil {
		return &maxItem.(*priceLevel).price
	}
	return nil
}

func (book *Orderbook) GetBidPricesDesc() []decimal.Decimal {
	book.lock.Lock()
	defer book.lock.Unlock()

	prices := make([]decimal.Decimal, 0)
	for key := range book.bidPrices {
		price, _ := decimal.NewFromString(key)
		prices = append(prices, price)
	}

	sort.Slice(prices, func(i, j int) bool {
		return prices[i].GreaterThan(prices[j])
	})

	return prices
}

func (book *Orderbook) GetAskPricesAsc() []decimal.Decimal {
	book.lock.Lock()
	defer book.lock.Unlock()

	prices := make([]decimal.Decimal, 0)
	for key := range book.askPrices {
		price, _ := decimal.NewFromString(key)
		prices = append(prices, price)
	}

	sort.Slice(prices, func(i, j int) bool {
		return prices[i].LessThan(prices[j])
	})

	return prices
}

// MinAsk ...
func (book *Orderbook) MinAsk() *decimal.Decimal {
	book.lock.Lock()
	defer book.lock.Unlock()

	minItem := book.asksTree.Min()

	if minItem != nil {
		return &minItem.(*priceLevel).price
	}

	return nil
}

// MinBid ...
func (book *Orderbook) MinBid() *decimal.Decimal {
	book.lock.Lock()
	defer book.lock.Unlock()

	maxItem := book.bidsTree.Min()
	if maxItem != nil {
		return &maxItem.(*priceLevel).price
	}
	return nil
}

// MaxAsk ...
func (book *Orderbook) MaxAsk() *decimal.Decimal {
	book.lock.Lock()
	defer book.lock.Unlock()

	minItem := book.asksTree.Max()

	if minItem != nil {
		return &minItem.(*priceLevel).price
	}

	return nil
}

func (book *Orderbook) GetAskOrdersByPrice(price decimal.Decimal) (orders []*MemoryOrder) {
	book.lock.Lock()
	defer book.lock.Unlock()

	orders = make([]*MemoryOrder, 0)

	var tree *llrb.LLRB
	tree = book.asksTree

	pl := tree.Get(newPriceLevel(price))

	if pl == nil {
		return
	}

	orders = pl.(*priceLevel).GetOrders()
	return
}

func (book *Orderbook) GetBidOrdersByPrice(price decimal.Decimal) (orders []*MemoryOrder) {
	book.lock.Lock()
	defer book.lock.Unlock()

	orders = make([]*MemoryOrder, 0)

	var tree *llrb.LLRB
	tree = book.bidsTree

	pl := tree.Get(newPriceLevel(price))

	if pl == nil {
		return
	}

	orders = pl.(*priceLevel).GetOrders()
	return
}
