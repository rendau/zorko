package dto

import (
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"

	domainModel "github.com/rendau/zorko/internal/domain/project_key/model"
	proto "github.com/rendau/zorko/pkg/proto/zorko_v1"
)

// domain → proto

func EncodeProjectKeyMain(v *domainModel.Main, _ int) *proto.ProjectKeyMain {
	if v == nil {
		return nil
	}
	return &proto.ProjectKeyMain{
		Id:              v.Id,
		CreatedAt:       timestamppb.New(v.CreatedAt),
		ModifiedAt:      timestamppb.New(v.ModifiedAt),
		ProjectId:       v.ProjectId,
		PublicKey:       v.PublicKey,
		Label:           v.Label,
		Active:          v.Active,
		RateLimitPerSec: v.RateLimitPerSec,
		AllowedOrigin:   v.AllowedOrigin,
		Dsn:             v.Dsn,
	}
}

// proto → domain

func DecodeProjectKeyListReq(v *proto.ProjectKeyListReq) *domainModel.ListReq {
	return &domainModel.ListReq{
		ListParams: DecodeListParams(v.GetListParams()),
		ProjectId:  v.ProjectId,
		Active:     v.Active,
		IDs:        lo.EmptyableToPtr(v.GetIds()),
	}
}

func DecodeProjectKeyCreateReq(v *proto.ProjectKeyCreateReq) *domainModel.Edit {
	return &domainModel.Edit{
		ProjectId:       v.ProjectId,
		Label:           v.Label,
		Active:          v.Active,
		RateLimitPerSec: v.RateLimitPerSec,
		AllowedOrigin:   decodeProjectKeyOriginList(v.GetAllowedOrigin()),
	}
}

func DecodeProjectKeyUpdateReq(v *proto.ProjectKeyUpdateReq) *domainModel.Edit {
	return &domainModel.Edit{
		Label:           v.Label,
		Active:          v.Active,
		RateLimitPerSec: v.RateLimitPerSec,
		AllowedOrigin:   decodeProjectKeyOriginList(v.GetAllowedOrigin()),
	}
}

// decodeProjectKeyOriginList — обёртка не передана → nil (не менять); передана пустой → пустой список
func decodeProjectKeyOriginList(v *proto.ProjectKeyOriginList) *[]string {
	if v == nil {
		return nil
	}
	return new(lo.Ternary(v.GetValues() == nil, []string{}, v.GetValues()))
}
