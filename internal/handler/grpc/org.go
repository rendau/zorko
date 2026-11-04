package grpc

import (
	"context"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rendau/zorko/internal/handler/grpc/dto"
	usecase "github.com/rendau/zorko/internal/usecase/org"
	"github.com/rendau/zorko/pkg/proto/common"
	proto "github.com/rendau/zorko/pkg/proto/zorko_v1"
)

type Org struct {
	proto.UnsafeOrgServiceServer
	usecase *usecase.Usecase
}

func NewOrg(uc *usecase.Usecase) *Org {
	return &Org{usecase: uc}
}

func (h *Org) List(ctx context.Context, req *proto.OrgListReq) (*proto.OrgListRep, error) {
	items, tCount, err := h.usecase.List(ctx, dto.DecodeOrgListReq(req))
	if err != nil {
		return nil, err
	}
	return &proto.OrgListRep{
		PaginationInfo: &common.PaginationInfoSt{
			Page:       req.GetListParams().GetPage(),
			PageSize:   req.GetListParams().GetPageSize(),
			TotalCount: tCount,
		},
		Results: lo.Map(items, dto.EncodeOrgMain),
	}, nil
}

func (h *Org) Get(ctx context.Context, req *proto.OrgGetReq) (*proto.OrgMain, error) {
	item, err := h.usecase.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return dto.EncodeOrgMain(item, 0), nil
}

func (h *Org) Create(ctx context.Context, req *proto.OrgCreateReq) (*proto.OrgCreateRep, error) {
	newId, err := h.usecase.Create(ctx, dto.DecodeOrgCreateReq(req))
	if err != nil {
		return nil, err
	}
	return &proto.OrgCreateRep{Id: newId}, nil
}

func (h *Org) Update(ctx context.Context, req *proto.OrgUpdateReq) (*emptypb.Empty, error) {
	if err := h.usecase.Update(ctx, req.GetId(), dto.DecodeOrgUpdateReq(req)); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (h *Org) Delete(ctx context.Context, req *proto.OrgGetReq) (*emptypb.Empty, error) {
	if err := h.usecase.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
