package api

import (
	"github.com/mcdexio/mai3-broker/common/mai3"
	"github.com/mcdexio/mai3-broker/conf"
	"github.com/mcdexio/mai3-broker/dao"
	"strings"
)

func (s *Server) GetPerpetual(p Param) (interface{}, error) {
	params := p.(*GetPerpetualReq)
	perpetual, err := s.dao.GetPerpetualByPoolAddressAndIndex(params.LiquidityPoolAddress, params.PerpetualIndex, true)
	if err != nil {
		if dao.IsRecordNotFound(err) {
			return nil, PerpetualNotFoundError(params.LiquidityPoolAddress, params.PerpetualIndex)
		}
		return nil, InternalError(err)
	}
	return &GetPerpetualResp{
		Perpetual: perpetual,
	}, nil
}

func (s *Server) GetBrokerRelay(p Param) (interface{}, error) {
	params := p.(*GetBrokerRelayReq)
	_, err := s.dao.GetPerpetualByPoolAddressAndIndex(params.LiquidityPoolAddress, params.PerpetualIndex, true)
	if err != nil {
		if dao.IsRecordNotFound(err) {
			return nil, PerpetualNotFoundError(params.LiquidityPoolAddress, params.PerpetualIndex)
		}
		return nil, InternalError(err)
	}

	relayer, err := s.chainCli.GetSignAccount()
	if err != nil {
		return nil, InternalError(err)
	}

	return &GetBrokerRelayResp{
		BrokerAddress:  strings.ToLower(conf.Conf.BrokerAddress),
		RelayerAddress: strings.ToLower(relayer),
		Version:        int(mai3.ProtocolV3),
	}, nil
}
