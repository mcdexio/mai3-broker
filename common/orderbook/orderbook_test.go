package orderbook

import (
	"github.com/mcdexio/mai3-broker/common/model"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type orderbookTestSuite struct {
	suite.Suite
	book *Orderbook
}

func (s *orderbookTestSuite) SetupSuite() {
}

func (s *orderbookTestSuite) SetupTest() {
	s.book = NewOrderbook()
}

func (s *orderbookTestSuite) TearDownTest() {
}

func (s *orderbookTestSuite) TearDownSuite() {
}

func (s *orderbookTestSuite) TestNewOrderbok() {
	s.Equal(0, s.book.bidsTree.Len())
	s.Equal(0, s.book.asksTree.Len())
	s.Nil(s.book.MaxBid())
	s.Nil(s.book.MinAsk())
}

func (s *orderbookTestSuite) TestInsertAndRemoveOrder() {
	s.book.InsertOrder(NewLimitOrder("o1", "1.2", "1"))
	s.book.InsertOrder(NewLimitOrder("o2", "1.2", "2"))
	s.book.InsertOrder(NewLimitOrder("o3", "-1.8", "4"))

	s.Equal(1, s.book.bidsTree.Len())
	s.Equal(1, s.book.asksTree.Len())

	maxBidPriceLevel := s.book.bidsTree.Max().(*priceLevel)
	s.Equal(2, maxBidPriceLevel.Len())
	s.Equal("3", maxBidPriceLevel.totalAmount.String())

	s.book.RemoveOrder(NewLimitOrder("o1", "-1.2", "1"))
	s.Equal(1, maxBidPriceLevel.Len())
	s.Equal("2", maxBidPriceLevel.totalAmount.String())
}

func (s *orderbookTestSuite) TestInsertAndChangeOrder() {
	s.book.InsertOrder(NewLimitOrder("o1", "1.2", "1"))
	s.book.InsertOrder(NewLimitOrder("o2", "1.2", "2"))
	s.book.InsertOrder(NewLimitOrder("o3", "-1.8", "4"))

	s.Equal(1, s.book.bidsTree.Len())
	s.Equal(1, s.book.asksTree.Len())

	maxBidPriceLevel := s.book.bidsTree.Max().(*priceLevel)
	s.Equal(2, maxBidPriceLevel.Len())
	s.Equal("3", maxBidPriceLevel.totalAmount.String())

	o1Price, _ := decimal.NewFromString("1.2")
	o, ok := s.book.GetOrder("o1", false, o1Price)
	s.True(ok)

	d1, _ := decimal.NewFromString("0.9")
	s.book.ChangeOrder(NewLimitOrder("o1", "1.2", "1"), d1)
	s.Equal(2, maxBidPriceLevel.Len())
	s.Equal("3.9", maxBidPriceLevel.totalAmount.String())

	s.Equal("1.9", o.Amount.String())

	d2, _ := decimal.NewFromString("-1")
	s.book.ChangeOrder(NewLimitOrder("o1", "1.2", "1"), d2)
	s.Equal(2, maxBidPriceLevel.Len())
	s.Equal("2.9", maxBidPriceLevel.totalAmount.String())
	s.Equal("0.9", o.Amount.String())

	err := s.book.ChangeOrder(NewLimitOrder("o1", "1.2", "1"), d2)
	s.NotNil(err)
	s.Equal(2, maxBidPriceLevel.Len())
	s.Equal("2.9", maxBidPriceLevel.totalAmount.String())
	s.Equal("0.9", o.Amount.String())

	d3, _ := decimal.NewFromString("-0.9")
	s.book.ChangeOrder(NewLimitOrder("o1", "1.2", "1"), d3)
	s.Equal(1, maxBidPriceLevel.Len())
	s.Equal("2", maxBidPriceLevel.totalAmount.String())
	s.Equal("0", o.Amount.String())
	o, ok = s.book.GetOrder("o1", false, o1Price)
	s.False(ok)
}

func TestOrderbookTestSuite(t *testing.T) {
	suite.Run(t, new(orderbookTestSuite))
}

type orderTestSuite struct {
	suite.Suite
}

func (s *orderTestSuite) SetupSuite() {
}

func (s *orderTestSuite) TearDownSuite() {
}

func (s *orderTestSuite) TearDownTest() {
}

func TestOrderTestSuite(t *testing.T) {
	suite.Run(t, new(orderTestSuite))
}

// NewLimitOrder ...
func NewLimitOrder(id string, price string, amount string) *MemoryOrder {
	return NewOrder(id, price, amount, model.LimitOrder)
}

func NewOrder(id, price, amount string, _type model.OrderType) *MemoryOrder {

	amountDecimal, _ := decimal.NewFromString(amount)
	priceDecimal, _ := decimal.NewFromString(price)

	return &MemoryOrder{
		ID:     id,
		Price:  priceDecimal,
		Amount: amountDecimal,
		Type:   _type,
	}
}
