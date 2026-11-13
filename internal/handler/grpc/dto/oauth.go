package dto

import (
	"net/url"

	"google.golang.org/protobuf/types/known/timestamppb"

	domainModel "github.com/rendau/zorko/internal/domain/oauth/model"
	usecase "github.com/rendau/zorko/internal/usecase/oauth"
	proto "github.com/rendau/zorko/pkg/proto/zorko_v1"
)

func DecodeOAuthAuthorizeReq(v *proto.OAuthAuthorizeReq) *usecase.AuthorizeReq {
	return &usecase.AuthorizeReq{
		ResponseType:        v.GetResponseType(),
		ClientId:            v.GetClientId(),
		RedirectUri:         v.GetRedirectUri(),
		State:               v.GetState(),
		CodeChallenge:       v.GetCodeChallenge(),
		CodeChallengeMethod: v.GetCodeChallengeMethod(),
		Scope:               v.GetScope(),
		Resource:            v.GetResource(),
	}
}

func EncodeOAuthAuthorizeInfo(v *usecase.AuthorizeInfo) *proto.OAuthAuthorizeInfoRep {
	return &proto.OAuthAuthorizeInfoRep{
		ClientName:   v.ClientName,
		RedirectUri:  v.RedirectUri,
		RedirectHost: v.RedirectHost,
	}
}

func EncodeOAuthGrantMain(v *domainModel.Grant, _ int) *proto.OAuthGrantMain {
	if v == nil {
		return nil
	}
	result := &proto.OAuthGrantMain{
		Id:          v.Id,
		ClientName:  v.ClientName,
		CreatedAt:   timestamppb.New(v.CreatedAt),
		ConnectedAt: encodeTimePtr(v.ConnectedAt),
		LastUsedAt:  encodeTimePtr(v.LastUsedAt),
	}
	if parsed, err := url.Parse(v.RedirectUri); err == nil {
		result.RedirectHost = parsed.Host
	}
	return result
}
