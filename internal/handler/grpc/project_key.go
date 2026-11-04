package grpc

import (
	"context"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rendau/zorko/internal/handler/grpc/dto"
	usecase "github.com/rendau/zorko/internal/usecase/project_key"
	"github.com/rendau/zorko/pkg/proto/common"
	proto "github.com/rendau/zorko/pkg/proto/zorko_v1"
)

type ProjectKey struct {
	proto.UnsafeProjectKeyServiceServer
	usecase *usecase.Usecase
}

func NewProjectKey(uc *usecase.Usecase) *ProjectKey {
	return &ProjectKey{usecase: uc}
}

func (h *ProjectKey) List(ctx context.Context, req *proto.ProjectKeyListReq) (*proto.ProjectKeyListRep, error) {
	items, tCount, err := h.usecase.List(ctx, dto.DecodeProjectKeyListReq(req))
	if err != nil {
		return nil, err
	}
	return &proto.ProjectKeyListRep{
		PaginationInfo: &common.PaginationInfoSt{
			Page:       req.GetListParams().GetPage(),
			PageSize:   req.GetListParams().GetPageSize(),
			TotalCount: tCount,
		},
		Results: lo.Map(items, dto.EncodeProjectKeyMain),
	}, nil
}

func (h *ProjectKey) Get(ctx context.Context, req *proto.ProjectKeyGetReq) (*proto.ProjectKeyMain, error) {
	item, err := h.usecase.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return dto.EncodeProjectKeyMain(item, 0), nil
}

func (h *ProjectKey) Create(ctx context.Context, req *proto.ProjectKeyCreateReq) (*proto.ProjectKeyCreateRep, error) {
	newId, err := h.usecase.Create(ctx, dto.DecodeProjectKeyCreateReq(req))
	if err != nil {
		return nil, err
	}
	return &proto.ProjectKeyCreateRep{Id: newId}, nil
}

func (h *ProjectKey) Update(ctx context.Context, req *proto.ProjectKeyUpdateReq) (*emptypb.Empty, error) {
	if err := h.usecase.Update(ctx, req.GetId(), dto.DecodeProjectKeyUpdateReq(req)); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (h *ProjectKey) Delete(ctx context.Context, req *proto.ProjectKeyGetReq) (*emptypb.Empty, error) {
	if err := h.usecase.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
