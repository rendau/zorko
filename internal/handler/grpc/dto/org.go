package dto

import (
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"

	domainModel "github.com/rendau/zorko/internal/domain/org/model"
	proto "github.com/rendau/zorko/pkg/proto/zorko_v1"
)

// domain → proto

func EncodeOrgMain(v *domainModel.Main, _ int) *proto.OrgMain {
	if v == nil {
		return nil
	}
	return &proto.OrgMain{
		Id:         v.Id,
		CreatedAt:  timestamppb.New(v.CreatedAt),
		ModifiedAt: timestamppb.New(v.ModifiedAt),
		Slug:       v.Slug,
		Name:       v.Name,
	}
}

// proto → domain

func DecodeOrgListReq(v *proto.OrgListReq) *domainModel.ListReq {
	return &domainModel.ListReq{
		ListParams: DecodeListParams(v.GetListParams()),
		Slug:       v.Slug,
		IDs:        lo.EmptyableToPtr(v.GetIds()),
	}
}

func DecodeOrgCreateReq(v *proto.OrgCreateReq) *domainModel.Edit {
	return &domainModel.Edit{
		Slug: v.Slug,
		Name: v.Name,
	}
}

func DecodeOrgUpdateReq(v *proto.OrgUpdateReq) *domainModel.Edit {
	return &domainModel.Edit{
		Slug: v.Slug,
		Name: v.Name,
	}
}
