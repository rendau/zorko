package dto

import (
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventModel "github.com/rendau/zorko/internal/domain/event/model"
	issueModel "github.com/rendau/zorko/internal/domain/issue/model"
	usecase "github.com/rendau/zorko/internal/usecase/issue"
	proto "github.com/rendau/zorko/pkg/proto/zorko_v1"
)

// domain → proto

func EncodeIssueMain(v *issueModel.Main) *proto.IssueMain {
	if v == nil {
		return nil
	}
	return &proto.IssueMain{
		Id:           v.Id,
		CreatedAt:    timestamppb.New(v.CreatedAt),
		ProjectId:    v.ProjectId,
		Title:        v.Title,
		Culprit:      v.Culprit,
		Type:         v.Type,
		Value:        v.Value,
		Level:        v.Level,
		Platform:     v.Platform,
		Status:       v.Status,
		FirstSeen:    timestamppb.New(v.FirstSeen),
		LastSeen:     timestamppb.New(v.LastSeen),
		TimesSeen:    v.TimesSeen,
		FirstRelease: v.FirstRelease,
		LastRelease:  v.LastRelease,
	}
}

func EncodeIssueListItem(v *usecase.ListItem, _ int) *proto.IssueListItem {
	return &proto.IssueListItem{
		Issue:     EncodeIssueMain(v.Issue),
		Events:    v.Events,
		Users:     v.Users,
		Sparkline: v.Sparkline,
	}
}

func EncodeIssueStatPoint(v *usecase.StatPoint, _ int) *proto.IssueStatPoint {
	return &proto.IssueStatPoint{Ts: timestamppb.New(v.Ts), Count: v.Count}
}

func EncodeTagDist(v *eventModel.TagDist, _ int) *proto.TagDist {
	return &proto.TagDist{
		Key:   v.Key,
		Total: v.Total,
		Values: lo.Map(v.Values, func(c eventModel.TagValueCount, _ int) *proto.TagValueCount {
			return &proto.TagValueCount{Value: c.Value, Count: c.Count}
		}),
	}
}

func EncodeEventSummary(v *eventModel.EventSummary, _ int) *proto.EventSummary {
	return &proto.EventSummary{
		EventId:     v.EventId,
		Timestamp:   timestamppb.New(v.Timestamp),
		Level:       v.Level,
		Release:     v.Release,
		Environment: v.Environment,
		Browser:     v.Browser,
		Os:          v.Os,
		Device:      v.Device,
		UserId:      v.UserId,
		UserEmail:   v.UserEmail,
		UserIp:      v.UserIp,
		Url:         v.Url,
		Title:       v.Title,
		Message:     v.Message,
	}
}

func EncodeEventMain(v *eventModel.EventDetail) *proto.EventMain {
	return &proto.EventMain{
		Summary:     EncodeEventSummary(&v.EventSummary, 0),
		Data:        v.Data,
		PrevEventId: v.PrevEventId,
		NextEventId: v.NextEventId,
	}
}

// proto → domain

func DecodeIssueListReq(v *proto.IssueListReq) *usecase.ListReq {
	return &usecase.ListReq{
		ListParams: DecodeListParams(v.GetListParams()),
		Scope: usecase.Scope{
			OrgId:       v.GetOrgId(),
			ProjectIds:  v.GetProjectIds(),
			Environment: v.GetEnvironment(),
			Period:      v.GetPeriod(),
		},
		Query: v.GetQ(),
		Sort:  v.GetSort(),
	}
}
