package grpc

import (
	"context"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rendau/zorko/internal/handler/grpc/dto"
	usecase "github.com/rendau/zorko/internal/usecase/project"
	"github.com/rendau/zorko/pkg/proto/common"
	proto "github.com/rendau/zorko/pkg/proto/zorko_v1"
)

type Project struct {
	proto.UnsafeProjectServiceServer
	usecase *usecase.Usecase
}

func NewProject(uc *usecase.Usecase) *Project {
	return &Project{usecase: uc}
}

func (h *Project) List(ctx context.Context, req *proto.ProjectListReq) (*proto.ProjectListRep, error) {
	items, tCount, err := h.usecase.List(ctx, dto.DecodeProjectListReq(req))
	if err != nil {
		return nil, err
	}
	return &proto.ProjectListRep{
		PaginationInfo: &common.PaginationInfoSt{
			Page:       req.GetListParams().GetPage(),
			PageSize:   req.GetListParams().GetPageSize(),
			TotalCount: tCount,
		},
		Results: lo.Map(items, dto.EncodeProjectMain),
	}, nil
}

func (h *Project) Get(ctx context.Context, req *proto.ProjectGetReq) (*proto.ProjectMain, error) {
	item, err := h.usecase.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return dto.EncodeProjectMain(item, 0), nil
}

func (h *Project) Create(ctx context.Context, req *proto.ProjectCreateReq) (*proto.ProjectCreateRep, error) {
	newId, err := h.usecase.Create(ctx, dto.DecodeProjectCreateReq(req))
	if err != nil {
		return nil, err
	}
	return &proto.ProjectCreateRep{Id: newId}, nil
}

func (h *Project) Update(ctx context.Context, req *proto.ProjectUpdateReq) (*emptypb.Empty, error) {
	if err := h.usecase.Update(ctx, req.GetId(), dto.DecodeProjectUpdateReq(req)); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (h *Project) Delete(ctx context.Context, req *proto.ProjectGetReq) (*emptypb.Empty, error) {
	if err := h.usecase.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
