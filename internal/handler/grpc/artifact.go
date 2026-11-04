package grpc

import (
	"context"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rendau/zorko/internal/handler/grpc/dto"
	usecase "github.com/rendau/zorko/internal/usecase/artifact"
	"github.com/rendau/zorko/pkg/proto/common"
	proto "github.com/rendau/zorko/pkg/proto/zorko_v1"
)

type ArtifactBundle struct {
	proto.UnsafeArtifactBundleServiceServer
	usecase *usecase.Usecase
}

func NewArtifactBundle(uc *usecase.Usecase) *ArtifactBundle {
	return &ArtifactBundle{usecase: uc}
}

func (h *ArtifactBundle) List(ctx context.Context, req *proto.ArtifactBundleListReq) (*proto.ArtifactBundleListRep, error) {
	items, total, err := h.usecase.ListBundles(ctx, dto.DecodeArtifactBundleListReq(req))
	if err != nil {
		return nil, err
	}
	return &proto.ArtifactBundleListRep{
		PaginationInfo: &common.PaginationInfoSt{
			Page:       req.GetListParams().GetPage(),
			PageSize:   req.GetListParams().GetPageSize(),
			TotalCount: total,
		},
		Results: lo.Map(items, dto.EncodeArtifactBundleMain),
	}, nil
}

func (h *ArtifactBundle) ListFiles(ctx context.Context, req *proto.ArtifactBundleGetReq) (*proto.ArtifactBundleFileListRep, error) {
	files, err := h.usecase.ListFiles(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &proto.ArtifactBundleFileListRep{Results: lo.Map(files, dto.EncodeArtifactBundleFile)}, nil
}

func (h *ArtifactBundle) Delete(ctx context.Context, req *proto.ArtifactBundleGetReq) (*emptypb.Empty, error) {
	if err := h.usecase.DeleteBundle(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

type SiteFetch struct {
	proto.UnsafeSiteFetchServiceServer
	usecase *usecase.Usecase
}

func NewSiteFetch(uc *usecase.Usecase) *SiteFetch {
	return &SiteFetch{usecase: uc}
}

func (h *SiteFetch) List(ctx context.Context, req *proto.SiteFetchListReq) (*proto.SiteFetchListRep, error) {
	items, err := h.usecase.ListSiteFetch(ctx, req.GetProjectId(), req.GetState())
	if err != nil {
		return nil, err
	}
	return &proto.SiteFetchListRep{Results: lo.Map(items, dto.EncodeSiteFetchMain)}, nil
}
