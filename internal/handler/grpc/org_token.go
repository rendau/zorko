package grpc

import (
	"context"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rendau/zorko/internal/handler/grpc/dto"
	usecase "github.com/rendau/zorko/internal/usecase/org_token"
	proto "github.com/rendau/zorko/pkg/proto/zorko_v1"
)

type OrgToken struct {
	proto.UnsafeOrgTokenServiceServer
	usecase *usecase.Usecase
}

func NewOrgToken(uc *usecase.Usecase) *OrgToken {
	return &OrgToken{usecase: uc}
}

func (h *OrgToken) List(ctx context.Context, req *proto.OrgTokenListReq) (*proto.OrgTokenListRep, error) {
	items, err := h.usecase.List(ctx, req.GetOrgId())
	if err != nil {
		return nil, err
	}
	return &proto.OrgTokenListRep{Results: lo.Map(items, dto.EncodeOrgTokenMain)}, nil
}

func (h *OrgToken) Create(ctx context.Context, req *proto.OrgTokenCreateReq) (*proto.OrgTokenCreateRep, error) {
	item, err := h.usecase.Create(ctx, req.GetOrgId(), req.GetName())
	if err != nil {
		return nil, err
	}
	return &proto.OrgTokenCreateRep{Item: dto.EncodeOrgTokenMain(&item.Main, 0), Token: item.Token}, nil
}

func (h *OrgToken) Delete(ctx context.Context, req *proto.OrgTokenDeleteReq) (*emptypb.Empty, error) {
	if err := h.usecase.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
