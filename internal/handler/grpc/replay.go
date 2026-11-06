package grpc

import (
	"context"

	"github.com/samber/lo"

	"github.com/rendau/zorko/internal/handler/grpc/dto"
	usecase "github.com/rendau/zorko/internal/usecase/replay"
	"github.com/rendau/zorko/pkg/proto/common"
	proto "github.com/rendau/zorko/pkg/proto/zorko_v1"
)

type Replay struct {
	proto.UnsafeReplayServiceServer
	usecase *usecase.Usecase
}

func NewReplay(uc *usecase.Usecase) *Replay {
	return &Replay{usecase: uc}
}

func (h *Replay) List(ctx context.Context, req *proto.ReplayListReq) (*proto.ReplayListRep, error) {
	items, total, err := h.usecase.List(ctx, dto.DecodeReplayListReq(req))
	if err != nil {
		return nil, err
	}
	return &proto.ReplayListRep{
		PaginationInfo: &common.PaginationInfoSt{
			Page:       req.GetListParams().GetPage(),
			PageSize:   req.GetListParams().GetPageSize(),
			TotalCount: total,
		},
		Results: lo.Map(items, dto.EncodeReplaySummary),
	}, nil
}

func (h *Replay) Get(ctx context.Context, req *proto.ReplayGetReq) (*proto.ReplayGetRep, error) {
	rep, err := h.usecase.Get(ctx, req.GetProjectId(), req.GetId())
	if err != nil {
		return nil, err
	}
	return dto.EncodeReplayGetRep(rep), nil
}

func (h *Replay) IssueList(ctx context.Context, req *proto.ReplayIssueListReq) (*proto.ReplayListRep, error) {
	items, total, err := h.usecase.IssueReplays(ctx, req.GetIssueId(), dto.DecodeListParams(req.GetListParams()))
	if err != nil {
		return nil, err
	}
	return &proto.ReplayListRep{
		PaginationInfo: &common.PaginationInfoSt{
			Page:       req.GetListParams().GetPage(),
			PageSize:   req.GetListParams().GetPageSize(),
			TotalCount: total,
		},
		Results: lo.Map(items, dto.EncodeReplaySummary),
	}, nil
}
