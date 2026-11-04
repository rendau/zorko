package grpc

import (
	"context"

	"github.com/samber/lo"

	"github.com/rendau/zorko/internal/handler/grpc/dto"
	usecase "github.com/rendau/zorko/internal/usecase/issue"
	"github.com/rendau/zorko/pkg/proto/common"
	proto "github.com/rendau/zorko/pkg/proto/zorko_v1"
)

type Issue struct {
	proto.UnsafeIssueServiceServer
	usecase *usecase.Usecase
}

func NewIssue(uc *usecase.Usecase) *Issue {
	return &Issue{usecase: uc}
}

func (h *Issue) List(ctx context.Context, req *proto.IssueListReq) (*proto.IssueListRep, error) {
	rep, err := h.usecase.List(ctx, dto.DecodeIssueListReq(req))
	if err != nil {
		return nil, err
	}
	return &proto.IssueListRep{
		PaginationInfo: &common.PaginationInfoSt{
			Page:       req.GetListParams().GetPage(),
			PageSize:   req.GetListParams().GetPageSize(),
			TotalCount: rep.Total,
		},
		Results:   lo.Map(rep.Items, dto.EncodeIssueListItem),
		Truncated: rep.Truncated,
	}, nil
}

func (h *Issue) Get(ctx context.Context, req *proto.IssueGetReq) (*proto.IssueGetRep, error) {
	rep, err := h.usecase.Get(ctx, req.GetId(), req.GetPeriod(), req.GetEnvironment())
	if err != nil {
		return nil, err
	}
	return &proto.IssueGetRep{Issue: dto.EncodeIssueMain(rep.Issue), Events: rep.Events, Users: rep.Users}, nil
}

func (h *Issue) Stat(ctx context.Context, req *proto.IssueStatReq) (*proto.IssueStatRep, error) {
	points, err := h.usecase.Stat(ctx, req.GetId(), req.GetPeriod(), req.GetInterval(), req.GetEnvironment())
	if err != nil {
		return nil, err
	}
	return &proto.IssueStatRep{Points: lo.Map(points, dto.EncodeIssueStatPoint)}, nil
}

func (h *Issue) Tags(ctx context.Context, req *proto.IssueTagsReq) (*proto.IssueTagsRep, error) {
	tags, err := h.usecase.Tags(ctx, req.GetId(), req.GetPeriod(), req.GetEnvironment(), req.GetQ())
	if err != nil {
		return nil, err
	}
	return &proto.IssueTagsRep{Tags: lo.Map(tags, dto.EncodeTagDist)}, nil
}
