package grpc

import (
	"context"

	usecase "github.com/rendau/zorko/internal/usecase/issue"
	proto "github.com/rendau/zorko/pkg/proto/zorko_v1"
)

type Tag struct {
	proto.UnsafeTagServiceServer
	usecase *usecase.Usecase
}

func NewTag(uc *usecase.Usecase) *Tag {
	return &Tag{usecase: uc}
}

func (h *Tag) Keys(ctx context.Context, req *proto.TagKeysReq) (*proto.TagKeysRep, error) {
	keys, err := h.usecase.TagKeys(ctx, usecase.Scope{OrgId: req.GetOrgId(), ProjectIds: req.GetProjectIds(), Period: req.GetPeriod()})
	if err != nil {
		return nil, err
	}
	return &proto.TagKeysRep{Keys: keys}, nil
}

func (h *Tag) Values(ctx context.Context, req *proto.TagValuesReq) (*proto.TagValuesRep, error) {
	values, err := h.usecase.TagValues(ctx, usecase.Scope{OrgId: req.GetOrgId(), ProjectIds: req.GetProjectIds(), Period: req.GetPeriod()},
		req.GetKey(), req.GetPrefix())
	if err != nil {
		return nil, err
	}
	return &proto.TagValuesRep{Values: values}, nil
}
