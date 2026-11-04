package grpc

import (
	"context"
	"net"
	"strings"

	"github.com/samber/lo"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rendau/zorko/internal/handler/grpc/dto"
	usecase "github.com/rendau/zorko/internal/usecase/usr"
	"github.com/rendau/zorko/pkg/proto/common"
	proto "github.com/rendau/zorko/pkg/proto/zorko_v1"
)

type Usr struct {
	proto.UnsafeUsrServiceServer
	usecase *usecase.Usecase
}

func NewUsr(uc *usecase.Usecase) *Usr {
	return &Usr{usecase: uc}
}

func (h *Usr) List(ctx context.Context, req *proto.UsrListReq) (*proto.UsrListRep, error) {
	items, tCount, err := h.usecase.List(ctx, dto.DecodeUsrListReq(req))
	if err != nil {
		return nil, err
	}
	return &proto.UsrListRep{
		PaginationInfo: &common.PaginationInfoSt{
			Page:       req.GetListParams().GetPage(),
			PageSize:   req.GetListParams().GetPageSize(),
			TotalCount: tCount,
		},
		Results: lo.Map(items, dto.EncodeUsrMain),
	}, nil
}

func (h *Usr) Get(ctx context.Context, req *proto.UsrGetReq) (*proto.UsrMain, error) {
	item, err := h.usecase.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return dto.EncodeUsrMain(item, 0), nil
}

func (h *Usr) Create(ctx context.Context, req *proto.UsrCreateReq) (*proto.UsrCreateRep, error) {
	newId, err := h.usecase.Create(ctx, dto.DecodeUsrCreateReq(req))
	if err != nil {
		return nil, err
	}
	return &proto.UsrCreateRep{Id: newId}, nil
}

func (h *Usr) Update(ctx context.Context, req *proto.UsrUpdateReq) (*emptypb.Empty, error) {
	if err := h.usecase.Update(ctx, req.GetId(), dto.DecodeUsrUpdateReq(req)); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (h *Usr) Delete(ctx context.Context, req *proto.UsrGetReq) (*emptypb.Empty, error) {
	if err := h.usecase.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (h *Usr) Login(ctx context.Context, req *proto.UsrLoginReq) (*proto.UsrLoginRep, error) {
	pair, err := h.usecase.Login(ctx, req.GetUsername(), req.GetPassword(), clientIp(ctx))
	if err != nil {
		return nil, err
	}
	return dto.EncodeUsrTokenPair(pair), nil
}

func (h *Usr) RefreshToken(ctx context.Context, req *proto.UsrRefreshTokenReq) (*proto.UsrLoginRep, error) {
	pair, err := h.usecase.RefreshToken(ctx, req.GetRefreshToken())
	if err != nil {
		return nil, err
	}
	return dto.EncodeUsrTokenPair(pair), nil
}

func (h *Usr) BootstrapStatus(ctx context.Context, _ *emptypb.Empty) (*proto.UsrBootstrapStatusRep, error) {
	required, err := h.usecase.BootstrapStatus(ctx)
	if err != nil {
		return nil, err
	}
	return &proto.UsrBootstrapStatusRep{BootstrapRequired: required}, nil
}

func (h *Usr) GetProfile(ctx context.Context, _ *emptypb.Empty) (*proto.UsrMain, error) {
	item, err := h.usecase.GetProfile(ctx)
	if err != nil {
		return nil, err
	}
	return dto.EncodeUsrMain(item, 0), nil
}

func (h *Usr) UpdateProfile(ctx context.Context, req *proto.UsrUpdateProfileReq) (*emptypb.Empty, error) {
	if err := h.usecase.UpdateProfile(ctx, dto.DecodeUsrUpdateProfileReq(req)); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// clientIp — адрес клиента: x-forwarded-for от HTTP-шлюза (первый адрес), иначе адрес соединения
func clientIp(ctx context.Context) string {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get("x-forwarded-for"); len(values) > 0 {
			first, _, _ := strings.Cut(values[0], ",")
			if ip := net.ParseIP(strings.TrimSpace(first)); ip != nil {
				return ip.String()
			}
		}
	}
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		if host, _, err := net.SplitHostPort(p.Addr.String()); err == nil {
			return host
		}
	}
	return ""
}
