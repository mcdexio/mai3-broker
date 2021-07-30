package mai3

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/mcdexio/mai3-broker/common/mai3/crypto"
	"github.com/mcdexio/mai3-broker/common/mai3/utils"
	"github.com/mcdexio/mai3-broker/common/model"
	"github.com/shopspring/decimal"
)

var EIP712_DOMAIN_TYPEHASH []byte
var EIP712_MAI3_ORDER_TYPE []byte

func init() {
	EIP712_DOMAIN_TYPEHASH = crypto.Keccak256([]byte(`EIP712Domain(string name)`))
	EIP712_MAI3_ORDER_TYPE = crypto.Keccak256([]byte(`Order(address trader,address broker,address relayer,address referrer,address liquidityPool,int256 minTradeAmount,int256 amount,int256 limitPrice,int256 triggerPrice,uint256 chainID,uint64 expiredAt,uint32 perpetualIndex,uint32 brokerFeeLimit,uint32 flags,uint32 salt)`))
}

func addLeadingZero(data string, length int) string {
	if length <= len(data) {
		return data
	}
	return strings.Repeat("0", length-len(data)) + data
}

var (
	s256 = BigPow(2, 256)
)

// BigPow returns a ** b as a big integer.
func BigPow(a, b int64) *big.Int {
	r := big.NewInt(a)
	return r.Exp(r, big.NewInt(b), nil)
}

func encodeNumber(d decimal.Decimal) string {
	b := encodeNumberToByte(d)
	return addLeadingZero(utils.Bytes2Hex(b), 8*8)
}

func encodeNumberToByte(d decimal.Decimal) []byte {
	b := utils.MustDecimalToBigInt(utils.ToWad(d))
	if d.IsNegative() {
		b = new(big.Int).Add(s256, b)
	}
	return b.Bytes()
}

func GenerateOrderFlags(orderType model.OrderType, isCloseOnly bool) int {
	flags := 0x0
	if isCloseOnly {
		flags = flags | model.MASK_CLOSE_ONLY
	}
	if orderType == model.StopLimitOrder {
		flags = flags | model.MASK_STOP_LOSS_ORDER
	} else if orderType == model.TakeProfitOrder {
		flags = flags | model.MASK_TAKE_PROFIT_ORDER
	}

	flags = flags | model.MASK_USE_TARGET_LEVERAGE

	return flags
}

func GenerateOrderData(traderAddress, brokerAddress, relayerAddress, referrerAddress, poolAddress string,
	minTradeAmount, amount, price, triggerPrice decimal.Decimal, chainID int64,
	expiredAt, perpetualIndex, brokerFeeLimit, flags, salt int64, signType, v, r, s string) string {
	data := strings.Builder{}
	data.WriteString("0x")
	trader, err := utils.Hex2Bytes(traderAddress)
	if err != nil {
		return ""
	}
	data.WriteString(utils.Bytes2Hex(trader))
	broker, err := utils.Hex2Bytes(brokerAddress)
	if err != nil {
		return ""
	}
	data.WriteString(utils.Bytes2Hex(broker))
	relayer, err := utils.Hex2Bytes(relayerAddress)
	if err != nil {
		return ""
	}
	data.WriteString(utils.Bytes2Hex(relayer))
	referrer, err := utils.Hex2Bytes(referrerAddress)
	if err != nil {
		return ""
	}
	data.WriteString(utils.Bytes2Hex(referrer))
	pool, err := utils.Hex2Bytes(poolAddress)
	if err != nil {
		return ""
	}
	data.WriteString(utils.Bytes2Hex(pool))
	data.WriteString(encodeNumber(minTradeAmount))
	data.WriteString(encodeNumber(amount))
	data.WriteString(encodeNumber(price))
	data.WriteString(addLeadingZero(utils.Bytes2Hex(encodeNumberToByte(triggerPrice)), 8*8))

	data.WriteString(addLeadingZero(fmt.Sprintf("%x", chainID), 8*8))
	data.WriteString(addLeadingZero(fmt.Sprintf("%x", expiredAt), 8*2))
	data.WriteString(addLeadingZero(fmt.Sprintf("%x", perpetualIndex), 8))
	data.WriteString(addLeadingZero(fmt.Sprintf("%x", brokerFeeLimit), 8))
	data.WriteString(addLeadingZero(fmt.Sprintf("%x", flags), 8))
	data.WriteString(addLeadingZero(fmt.Sprintf("%x", salt), 8))
	data.WriteString(v)
	if signType == EIP712 {
		data.WriteString("01")
	} else {
		data.WriteString("00")
	}
	data.WriteString(r)
	data.WriteString(s)

	return data.String()
}

func GetOrderHash(traderAddress, brokerAddress, relayerAddress, referrerAddress, poolAddress string,
	minTradeAmount, amount, price, triggerPrice decimal.Decimal, chainID int64,
	expiredAt, perpetualIndex, brokerFeeLimit, flags, salt int64) ([]byte, error) {
	trader, err := utils.HexToHash(traderAddress)
	if err != nil {
		return nil, fmt.Errorf("GetOrderHash:%w", err)
	}
	broker, err := utils.HexToHash(brokerAddress)
	if err != nil {
		return nil, fmt.Errorf("GetOrderHash:%w", err)
	}
	relayer, err := utils.HexToHash(relayerAddress)
	if err != nil {
		return nil, fmt.Errorf("GetOrderHash:%w", err)
	}

	referrer, err := utils.HexToHash(referrerAddress)
	if err != nil {
		return nil, fmt.Errorf("GetOrderHash:%w", err)
	}

	pool, err := utils.HexToHash(poolAddress)
	if err != nil {
		return nil, fmt.Errorf("GetOrderHash:%w", err)
	}

	minTradeAmountBin := utils.BytesToHash(utils.MustDecimalToBigInt(utils.ToWad(minTradeAmount)).Bytes())
	amountBin := utils.BytesToHash(encodeNumberToByte(amount))
	priceBin := utils.BytesToHash(utils.MustDecimalToBigInt(utils.ToWad(price)).Bytes())
	triggerPriceBin := utils.BytesToHash(utils.MustDecimalToBigInt(utils.ToWad(triggerPrice)).Bytes())
	chainIDBin := utils.BytesToHash(big.NewInt(chainID).Bytes())
	expiredAtBin := utils.BytesToHash(big.NewInt(expiredAt).Bytes())
	perpetualIndexBin := utils.BytesToHash(big.NewInt(perpetualIndex).Bytes())
	brokerFeeLimitBin := utils.BytesToHash(big.NewInt(brokerFeeLimit).Bytes())
	flagsBin := utils.BytesToHash(big.NewInt(flags).Bytes())
	saltBin := utils.BytesToHash(big.NewInt(salt).Bytes())

	hash := getEIP712MessageHash(
		crypto.Keccak256(
			EIP712_MAI3_ORDER_TYPE,
			trader.Bytes(),
			broker.Bytes(),
			relayer.Bytes(),
			referrer.Bytes(),
			pool.Bytes(),
			minTradeAmountBin.Bytes(),
			amountBin.Bytes(),
			priceBin.Bytes(),
			triggerPriceBin.Bytes(),
			chainIDBin.Bytes(),
			expiredAtBin.Bytes(),
			perpetualIndexBin.Bytes(),
			brokerFeeLimitBin.Bytes(),
			flagsBin.Bytes(),
			saltBin.Bytes(),
		),
	)
	return hash, nil
}

func GetGasFeeLimit(perpetualCount int, isCloseOnly bool) int64 {
	if isCloseOnly {
		return MaiV3BaseGasCloseOnly + MaiV3GasForEachPerpCloseOnly*int64(perpetualCount)
	}
	return MaiV3BaseGas + MaiV3GasForEachPerp*int64(perpetualCount)
}

func getDomainSeparator() []byte {
	return crypto.Keccak256(
		EIP712_DOMAIN_TYPEHASH,
		crypto.Keccak256([]byte("Mai Protocol v3")),
	)
}

func getEIP712MessageHash(message []byte) []byte {
	return crypto.Keccak256(
		[]byte{'\x19', '\x01'},
		getDomainSeparator(),
		message,
	)
}
