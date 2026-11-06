package dto

import (
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"

	replayModel "github.com/rendau/zorko/internal/domain/replay/model"
	usecase "github.com/rendau/zorko/internal/usecase/replay"
	proto "github.com/rendau/zorko/pkg/proto/zorko_v1"
)

// domain → proto

func EncodeReplaySummary(v *replayModel.Summary, _ int) *proto.ReplaySummary {
	if v == nil {
		return nil
	}
	result := &proto.ReplaySummary{
		Id:          v.ReplayId,
		ProjectId:   v.ProjectId,
		ReplayType:  v.Type,
		StartedAt:   timestamppb.New(v.Start),
		FinishedAt:  timestamppb.New(v.End),
		DurationMs:  max(v.End.Sub(v.Start).Milliseconds(), 0),
		Segments:    int32(v.Segments),
		MaxSegment:  int32(v.MaxSegment),
		Url:         v.Url,
		ErrorCount:  int32(v.ErrorCount),
		Environment: v.Environment,
		Release:     v.Release,
		UserId:      v.UserId,
		UserEmail:   v.UserEmail,
		UserIp:      v.UserIp,
		Browser:     v.Browser,
		Os:          v.Os,
		Device:      v.Device,
		SdkName:     v.SdkName,
		SdkVersion:  v.SdkVersion,
	}
	if v.ErrorAt != nil {
		result.ErrorAt = timestamppb.New(*v.ErrorAt)
	}
	return result
}

func EncodeReplayErrorMark(v *usecase.ErrorMark, _ int) *proto.ReplayErrorMark {
	return &proto.ReplayErrorMark{
		IssueId:   v.IssueId,
		Title:     v.Title,
		EventId:   v.EventId,
		Timestamp: timestamppb.New(v.Timestamp),
	}
}

func EncodeReplayGetRep(v *usecase.GetRep) *proto.ReplayGetRep {
	return &proto.ReplayGetRep{
		Summary: EncodeReplaySummary(v.Summary, 0),
		Errors:  lo.Map(v.Errors, EncodeReplayErrorMark),
	}
}

// proto → domain

func DecodeReplayListReq(v *proto.ReplayListReq) *usecase.ListReq {
	return &usecase.ListReq{
		ListParams:  DecodeListParams(v.GetListParams()),
		OrgId:       v.GetOrgId(),
		ProjectIds:  v.GetProjectIds(),
		Environment: v.GetEnvironment(),
		Release:     v.GetRelease(),
		Query:       v.GetQ(),
		HasErrors:   v.HasErrors,
		Type:        v.GetReplayType(),
	}
}
