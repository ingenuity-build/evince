package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	icstypes "github.com/ingenuity-build/quicksilver/x/interchainstaking/types"
	echov4 "github.com/labstack/echo/v4"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
)

func (s *Service) ConfigureRoutes() {
	s.Echo.GET("/", func(ctx echov4.Context) error {
		output := fmt.Sprintf("Quicksilver (evince): %v\n%v", GitCommit, LogoStr)
		return ctx.String(http.StatusOK, output)
	})

	s.Echo.GET("/validatorList/:chainId", func(ctx echov4.Context) error {
		chainID := ctx.Param("chainId")

		key := fmt.Sprintf("validatorList.%s", chainID)

		data, found := s.Cache.Get(key)
		if !found {
			return s.getValidatorList(ctx, key, chainID)
		}

		return ctx.JSONBlob(http.StatusOK, data.([]byte))
	})

	s.Echo.GET("/existingDelegations/:chainId/:address", func(c echov4.Context) error {
		chainID := c.Param("chainId")
		address := c.Param("address")

		key := fmt.Sprintf("existingDelegations.%s.%s", chainID, address)

		data, found := s.Cache.Get(key)
		if !found {
			return s.getExistingDelegations(c, key, chainID, address)
		}

		return c.JSONBlob(http.StatusOK, data.([]byte))
	})

	s.Echo.GET("/zones", func(ctx echov4.Context) error {
		key := "zones"

		data, found := s.Cache.Get(key)
		if !found {
			return s.getZones(ctx, key)
		}

		return ctx.JSONBlob(http.StatusOK, data.([]byte))
	})

	s.Echo.GET("/apr", func(ctx echov4.Context) error {
		key := "apr"

		data, found := s.Cache.Get(key)
		if !found {
			return s.getAPR(ctx, key)
		}
		return ctx.JSONBlob(http.StatusOK, data.([]byte))
	})

	s.Echo.GET("/total_supply", func(ctx echov4.Context) error {
		key := "total_supply"

		data, found := s.Cache.Get(key)
		if !found {
			return s.getTotalSupply(ctx, key)
		}
		return ctx.JSONBlob(http.StatusOK, data.([]byte))
	})

	s.Echo.GET("/circulating_supply", func(ctx echov4.Context) error {
		key := "circulating_supply"

		data, found := s.Cache.Get(key)
		if !found {
			return s.getCirculatingSupply(ctx, key)
		}
		return ctx.JSONBlob(http.StatusOK, data.([]byte))
	})
}

func (s *Service) getValidatorList(ctx echov4.Context, key, chainID string) error {
	s.Echo.Logger.Infof("getValidatorList")

	host := fmt.Sprintf(s.Config.ChainHost, chainID)

	// establish client connection
	client, err := NewRPCClient(host, 30*time.Second)
	if err != nil {
		s.Echo.Logger.Errorf("getValidatorList: %v - %v", ErrRPCClientConnection, err)
		return ErrRPCClientConnection
	}

	// prepare codecs
	interfaceRegistry := cdctypes.NewInterfaceRegistry()
	stakingtypes.RegisterInterfaces(interfaceRegistry)
	marshaler := codec.NewProtoCodec(interfaceRegistry)

	queryResponse := stakingtypes.QueryValidatorsResponse{}
	for i := 0; queryResponse.Pagination == nil || len(queryResponse.Validators) < int(queryResponse.Pagination.Total); i++ {
		// prepare query
		vQuery := stakingtypes.QueryValidatorsRequest{
			Status: "",
		}
		if queryResponse.Pagination != nil && len(queryResponse.Pagination.NextKey) > 0 {
			vQuery.Pagination = &query.PageRequest{
				Key: queryResponse.Pagination.NextKey,
			}
		}
		qBytes := marshaler.MustMarshal(&vQuery)

		// execute query
		abciQuery, err := client.ABCIQueryWithOptions(
			context.Background(),
			"/cosmos.staking.v1beta1.Query/Validators",
			qBytes,
			rpcclient.ABCIQueryOptions{Height: 0},
		)
		if err != nil {
			s.Echo.Logger.Errorf("getValidatorList: %v - %v", ErrABCIQuery, err)
			return ErrABCIQuery
		}

		// decode query response
		if err := marshaler.Unmarshal(abciQuery.Response.Value, &queryResponse); err != nil {
			s.Echo.Logger.Errorf("getValidatorList: %v - %v", ErrUnmarshalResponse, err)
			return ErrUnmarshalResponse
		}
	}

	// encode response & cache
	respData, err := codec.ProtoMarshalJSON(&queryResponse, nil)
	if err != nil {
		s.Echo.Logger.Errorf("getValidatorList: %v - %v", ErrMarshalResponse, err)
		return ErrMarshalResponse
	}

	s.Cache.SetWithTTL(key, respData, 1, 1*time.Hour)

	return ctx.JSONBlob(http.StatusOK, respData)
}

func (s *Service) getExistingDelegations(ctx echov4.Context, key, chainID, address string) error {
	s.Echo.Logger.Infof("getExistingDelegations")

	host := fmt.Sprintf(s.Config.ChainHost, chainID)

	// establish client connection
	client, err := NewRPCClient(host, 30*time.Second)
	if err != nil {
		s.Echo.Logger.Errorf("getExistingDelegations: %v - %v", ErrRPCClientConnection, err)
		return ErrRPCClientConnection
	}

	// prepare codecs
	interfaceRegistry := cdctypes.NewInterfaceRegistry()
	stakingtypes.RegisterInterfaces(interfaceRegistry)
	marshaler := codec.NewProtoCodec(interfaceRegistry)

	// prepare query
	queryRequest := stakingtypes.QueryDelegatorDelegationsRequest{
		DelegatorAddr: address,
	}
	qBytes := marshaler.MustMarshal(&queryRequest)

	// execute query
	abciQuery, err := client.ABCIQueryWithOptions(
		context.Background(),
		"/cosmos.staking.v1beta1.Query/DelegatorDelegations",
		qBytes,
		rpcclient.ABCIQueryOptions{Height: 0},
	)
	if err != nil {
		s.Echo.Logger.Errorf("getExistingDelegations: %v - %v", ErrABCIQuery, err)
		return ErrABCIQuery
	}

	// decode query response
	queryResponse := stakingtypes.QueryDelegatorDelegationsResponse{}
	if err := marshaler.Unmarshal(abciQuery.Response.Value, &queryResponse); err != nil {
		s.Echo.Logger.Errorf("getExistingDelegations: %v - %v", ErrUnmarshalResponse, err)
		return ErrUnmarshalResponse
	}

	// encode response & cache
	respData, err := marshaler.MarshalJSON(&queryResponse)
	if err != nil {
		s.Echo.Logger.Errorf("getExistingDelegations: %v - %v", ErrMarshalResponse, err)
		return ErrMarshalResponse
	}

	s.Cache.SetWithTTL(key, respData, 1, 2*time.Minute)

	return ctx.JSONBlob(http.StatusOK, respData)
}

func (s *Service) getZones(ctx echov4.Context, key string) error {
	s.Echo.Logger.Infof("getZones")

	// establish client connection
	client, err := NewRPCClient(s.Config.QuickHost, 30*time.Second)
	if err != nil {
		s.Echo.Logger.Errorf("getZones: %v - %v", ErrRPCClientConnection, err)
		return ErrRPCClientConnection
	}

	// prepare codecs
	interfaceRegistry := cdctypes.NewInterfaceRegistry()
	icstypes.RegisterInterfaces(interfaceRegistry)
	marshaler := codec.NewProtoCodec(interfaceRegistry)

	// prepare query
	queryRequest := icstypes.QueryZonesInfoRequest{}
	qBytes := marshaler.MustMarshal(&queryRequest)

	// execute query
	abciQuery, err := client.ABCIQueryWithOptions(
		context.Background(),
		"/quicksilver.interchainstaking.v1.Query/ZoneInfos",
		qBytes,
		rpcclient.ABCIQueryOptions{Height: 0},
	)
	if err != nil {
		s.Echo.Logger.Errorf("getZones: %v - %v", ErrABCIQuery, err)
		return ErrABCIQuery
	}

	// decode query response
	queryResponse := icstypes.QueryZonesInfoResponse{}
	if err := marshaler.Unmarshal(abciQuery.Response.Value, &queryResponse); err != nil {
		s.Echo.Logger.Errorf("getZones: %v - %v", ErrUnmarshalResponse, err)
		return ErrUnmarshalResponse
	}

	// encode response & cache
	respData, err := marshaler.MarshalJSON(&queryResponse)
	if err != nil {
		s.Echo.Logger.Errorf("getZones: %v - %v", ErrMarshalResponse, err)
		return ErrMarshalResponse
	}

	s.Cache.SetWithTTL(key, respData, 1, 1*time.Minute)

	return ctx.JSONBlob(http.StatusOK, respData)
}

func (s *Service) getAPR(ctx echov4.Context, key string) error {
	s.Echo.Logger.Infof("getAPR")

	chains := s.Config.Chains
	aprResp := APRResponse{}
	for _, chain := range chains {
		chainAPR, err := getAPRquery(context.Background(), s.Config.APRURL+"/", chain)
		if err != nil {
			s.Echo.Logger.Errorf("getAPR: %v - %v", ErrUnableToGetAPR, err)
			return ErrUnableToGetAPR
		}

		aprResp.Chains = append(aprResp.Chains, chainAPR)
	}

	respData, err := json.Marshal(aprResp)
	if err != nil {
		s.Echo.Logger.Errorf("getAPR: %v - %v", ErrMarshalResponse, err)
		return ErrMarshalResponse
	}

	s.Cache.SetWithTTL(key, respData, 1, time.Duration(s.Config.APRCacheTime)*time.Minute)

	return ctx.JSONBlob(http.StatusOK, respData)
}

func (s *Service) getTotalSupply(ctx echov4.Context, key string) error {
	s.Echo.Logger.Infof("getTotalSupply")

	totalSupply, err := getTotalSupply(context.Background(), s.Config.LCDEndpoint+"/cosmos/bank/v1beta1/supply")
	if err != nil {
		s.Echo.Logger.Errorf("getTotalSupply: %v - %v", ErrUnableToGetTotalSupply, err)
		return ErrUnableToGetTotalSupply
	}
	s.Echo.Logger.Info("totalSupply", " -> ", totalSupply)
	respData, err := json.Marshal(float64(totalSupply.Int64()) / 1_000_000)
	if err != nil {
		s.Echo.Logger.Errorf("getTotalSupply: %v - %v", ErrMarshalResponse, err)
		return ErrMarshalResponse
	}
	s.Cache.SetWithTTL(key, respData, 1, time.Duration(s.Config.SupplyCacheTime)*time.Hour)

	return ctx.JSONBlob(http.StatusOK, respData)
}

func (s *Service) getCirculatingSupply(ctx echov4.Context, key string) error {
	s.Echo.Logger.Infof("getCirculatingSupply")

	var CirculatingSupplyResponse int64

	totalLockedTokens := sdkmath.ZeroInt()

	for _, address := range VestingAccounts {
		lockedTokensForAddress, err := getVestingAccountLocked(context.Background(), s.Config.LCDEndpoint+"/cosmos/auth/v1beta1/accounts/", address)
		if err != nil {
			s.Echo.Logger.Errorf("getCirculatingSupply: %v - %v", ErrUnableToGetLockedTokens, err)
			return ErrUnableToGetLockedTokens
		}
		totalLockedTokens = totalLockedTokens.Add(lockedTokensForAddress)
		s.Echo.Logger.Info("lockedTokensFor", address, " -> ", lockedTokensForAddress)
	}

	totalSupply, err := getTotalSupply(context.Background(), s.Config.LCDEndpoint+"/cosmos/bank/v1beta1/supply")
	if err != nil {
		s.Echo.Logger.Errorf("getCirculatingSupply: %v - %v", ErrUnableToGetTotalSupply, err)
		return ErrUnableToGetTotalSupply
	}
	s.Echo.Logger.Info("totalSupply", " -> ", totalSupply)

	communityPoolBalance, err := getCommunityPool(context.Background(), s.Config.LCDEndpoint+"/cosmos/distribution/v1beta1/community_pool")
	if err != nil {
		s.Echo.Logger.Errorf("getCirculatingSupply: %v - %v", ErrUnableToGetCommunityPool, err)
		return ErrUnableToGetCommunityPool
	}

	s.Echo.Logger.Info("communityPoolBalance", " -> ", communityPoolBalance)

	totalCirculatingSupply := totalSupply.Sub(totalLockedTokens).Sub(communityPoolBalance).Sub(sdkmath.NewInt(500_000_000_000)) // unknown account
	CirculatingSupplyResponse = totalCirculatingSupply.Int64()

	respData, err := json.Marshal(float64(CirculatingSupplyResponse) / 1_000_000)
	if err != nil {
		s.Echo.Logger.Errorf("getCirculatingSupply: %v - %v", ErrMarshalResponse, err)
		return ErrMarshalResponse
	}
	s.Cache.SetWithTTL(key, respData, 1, time.Duration(s.Config.SupplyCacheTime)*time.Hour)

	return ctx.JSONBlob(http.StatusOK, respData)
}
