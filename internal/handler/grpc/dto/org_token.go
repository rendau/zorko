package dto

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	domainModel "github.com/rendau/zorko/internal/domain/org_token/model"
	proto "github.com/rendau/zorko/pkg/proto/zorko_v1"
)

func EncodeOrgTokenMain(v *domainModel.Main, _ int) *proto.OrgTokenMain {
	if v == nil {
		return nil
	}
	return &proto.OrgTokenMain{
		Id:            v.Id,
		CreatedAt:     timestamppb.New(v.CreatedAt),
		OrgId:         v.OrgId,
		CreatedByName: v.CreatedByName,
		Name:          v.Name,
		Hint:          v.Hint,
		LastUsedAt:    encodeTimePtr(v.LastUsedAt),
	}
}

// encodeTimePtr — нет времени → поле не передаётся
func encodeTimePtr(v *time.Time) *timestamppb.Timestamp {
	if v == nil {
		return nil
	}
	return timestamppb.New(*v)
}
