package dto

import (
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"

	domainModel "github.com/rendau/zorko/internal/domain/usr/model"
	usecase "github.com/rendau/zorko/internal/usecase/usr"
	proto "github.com/rendau/zorko/pkg/proto/zorko_v1"
)

// domain → proto (хеш пароля наружу не уходит)

func EncodeUsrMain(v *domainModel.Main, _ int) *proto.UsrMain {
	if v == nil {
		return nil
	}
	return &proto.UsrMain{
		Id:         v.Id,
		CreatedAt:  timestamppb.New(v.CreatedAt),
		ModifiedAt: timestamppb.New(v.ModifiedAt),
		Active:     v.Active,
		IsAdmin:    v.IsAdmin,
		Name:       v.Name,
		Username:   v.Username,
	}
}

func EncodeUsrTokenPair(v *usecase.TokenPair) *proto.UsrLoginRep {
	return &proto.UsrLoginRep{Jwt: v.Jwt, RefreshToken: v.RefreshToken}
}

// proto → domain

func DecodeUsrListReq(v *proto.UsrListReq) *domainModel.ListReq {
	return &domainModel.ListReq{
		ListParams: DecodeListParams(v.GetListParams()),
		Active:     v.Active,
		IDs:        lo.EmptyableToPtr(v.GetIds()),
	}
}

func DecodeUsrCreateReq(v *proto.UsrCreateReq) *domainModel.Edit {
	return &domainModel.Edit{
		Active:   v.Active,
		IsAdmin:  v.IsAdmin,
		Name:     v.Name,
		Username: v.Username,
		Password: v.Password,
	}
}

func DecodeUsrUpdateReq(v *proto.UsrUpdateReq) *domainModel.Edit {
	return &domainModel.Edit{
		Active:   v.Active,
		IsAdmin:  v.IsAdmin,
		Name:     v.Name,
		Username: v.Username,
		Password: v.Password,
	}
}

func DecodeUsrUpdateProfileReq(v *proto.UsrUpdateProfileReq) *usecase.UpdateProfileReq {
	return &usecase.UpdateProfileReq{
		Name:            v.Name,
		Username:        v.Username,
		Password:        v.Password,
		CurrentPassword: v.CurrentPassword,
	}
}
