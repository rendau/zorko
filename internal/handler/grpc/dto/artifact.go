package dto

import (
	"google.golang.org/protobuf/types/known/timestamppb"

	artifactModel "github.com/rendau/zorko/internal/domain/artifact/model"
	proto "github.com/rendau/zorko/pkg/proto/zorko_v1"
)

func EncodeArtifactBundleMain(v *artifactModel.Bundle, _ int) *proto.ArtifactBundleMain {
	return &proto.ArtifactBundleMain{
		Id:         v.Id,
		CreatedAt:  timestamppb.New(v.CreatedAt),
		Source:     v.Source,
		State:      v.State,
		Detail:     v.Detail,
		Release:    v.Release,
		Dist:       v.Dist,
		FileCount:  v.FileCount,
		Size:       v.Size,
		LastUsedAt: encodeTimePtr(v.LastUsedAt),
	}
}

func EncodeArtifactBundleFile(v *artifactModel.File, _ int) *proto.ArtifactBundleFile {
	return &proto.ArtifactBundleFile{Url: v.Url, Type: v.Type, DebugId: v.DebugId, Sourcemap: v.Sourcemap, Size: v.Size}
}

func DecodeArtifactBundleListReq(v *proto.ArtifactBundleListReq) *artifactModel.BundleListReq {
	return &artifactModel.BundleListReq{ListParams: DecodeListParams(v.GetListParams()), ProjectId: v.GetProjectId(), Query: v.GetQ()}
}

func EncodeSiteFetchMain(v *artifactModel.SiteFetch, _ int) *proto.SiteFetchMain {
	return &proto.SiteFetchMain{Url: v.Url, State: v.State, Detail: v.Detail, FetchedAt: timestamppb.New(v.FetchedAt)}
}
