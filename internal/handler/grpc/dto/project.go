package dto

import (
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"

	domainModel "github.com/rendau/zorko/internal/domain/project/model"
	proto "github.com/rendau/zorko/pkg/proto/zorko_v1"
)

// domain → proto

func EncodeProjectMain(v *domainModel.Main, _ int) *proto.ProjectMain {
	if v == nil {
		return nil
	}
	return &proto.ProjectMain{
		Id:         v.Id,
		CreatedAt:  timestamppb.New(v.CreatedAt),
		ModifiedAt: timestamppb.New(v.ModifiedAt),
		OrgId:      v.OrgId,
		Slug:       v.Slug,
		Name:       v.Name,
		Platform:   v.Platform,

		SiteFetchEnabled:        v.SiteFetchEnabled,
		SiteFetchHost:           v.SiteFetchHost,
		SiteFetchHeaderName:     v.SiteFetchHeaderName,
		SiteFetchHeaderValueSet: v.SiteFetchHeaderValue != "",
		SiteFetchIntervalMin:    v.SiteFetchIntervalMin,
	}
}

// proto → domain

func DecodeProjectListReq(v *proto.ProjectListReq) *domainModel.ListReq {
	return &domainModel.ListReq{
		ListParams: DecodeListParams(v.GetListParams()),
		OrgId:      v.OrgId,
		Slug:       v.Slug,
		IDs:        lo.EmptyableToPtr(v.GetIds()),
	}
}

func DecodeProjectCreateReq(v *proto.ProjectCreateReq) *domainModel.Edit {
	return &domainModel.Edit{
		OrgId:    v.OrgId,
		Slug:     v.Slug,
		Name:     v.Name,
		Platform: v.Platform,
	}
}

func DecodeProjectUpdateReq(v *proto.ProjectUpdateReq) *domainModel.Edit {
	result := &domainModel.Edit{
		Slug:                 v.Slug,
		Name:                 v.Name,
		Platform:             v.Platform,
		SiteFetchEnabled:     v.SiteFetchEnabled,
		SiteFetchHeaderName:  v.SiteFetchHeaderName,
		SiteFetchIntervalMin: v.SiteFetchIntervalMin,
	}
	if hosts := v.GetSiteFetchHost(); hosts != nil {
		result.SiteFetchHost = new(lo.Ternary(hosts.GetValues() == nil, []string{}, hosts.GetValues()))
	}
	switch {
	case v.GetSiteFetchHeaderClear():
		result.SiteFetchHeaderValue = new("")
	case v.GetSiteFetchHeaderValue() != "":
		result.SiteFetchHeaderValue = v.SiteFetchHeaderValue
	}
	return result
}
