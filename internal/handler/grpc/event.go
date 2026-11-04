package grpc

import (
	"context"

	"github.com/samber/lo"

	"github.com/rendau/zorko/internal/handler/grpc/dto"
	usecase "github.com/rendau/zorko/internal/usecase/issue"
	"github.com/rendau/zorko/pkg/proto/common"
	proto "github.com/rendau/zorko/pkg/proto/zorko_v1"
)

type Event struct {
	proto.UnsafeEventServiceServer
	usecase *usecase.Usecase
}

func NewEvent(uc *usecase.Usecase) *Event {
	return &Event{usecase: uc}
}

func (h *Event) List(ctx context.Context, req *proto.EventListReq) (*proto.EventListRep, error) {
	items, total, err := h.usecase.ListEvents(ctx, req.GetIssueId(), req.GetQ(), req.GetPeriod(), dto.DecodeListParams(req.GetListParams()))
	if err != nil {
		return nil, err
	}
	return &proto.EventListRep{
		PaginationInfo: &common.PaginationInfoSt{
			Page:       req.GetListParams().GetPage(),
			PageSize:   req.GetListParams().GetPageSize(),
			TotalCount: total,
		},
		Results: lo.Map(items, dto.EncodeEventSummary),
	}, nil
}

func (h *Event) Get(ctx context.Context, req *proto.EventGetReq) (*proto.EventMain, error) {
	item, err := h.usecase.GetEvent(ctx, req.GetIssueId(), req.GetEventId())
	if err != nil {
		return nil, err
	}
	return dto.EncodeEventMain(item), nil
}
