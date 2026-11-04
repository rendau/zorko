package dto

import (
	"encoding/json"

	"google.golang.org/protobuf/types/known/structpb"

	commonModel "github.com/rendau/zorko/internal/domain/common/model"
	"github.com/rendau/zorko/pkg/proto/common"
)

func DecodeListParams(v *common.ListParamsSt) commonModel.ListParams {
	if v == nil {
		return commonModel.ListParams{}
	}

	return commonModel.ListParams{
		Page:           v.Page,
		PageSize:       v.PageSize,
		WithTotalCount: v.WithTotalCount,
		OnlyCount:      v.OnlyCount,
		SortName:       v.SortName,
		Sort:           v.Sort,
	}
}

// JsonObjToGrpcStruct — JSON-объект (например, jsonb-колонка) → google.protobuf.Struct.
// Пустой или невалидный JSON → nil.
func JsonObjToGrpcStruct(v []byte) *structpb.Struct {
	if len(v) == 0 {
		return nil
	}

	obj := map[string]any{}
	if err := json.Unmarshal(v, &obj); err != nil {
		return nil
	}

	result, err := structpb.NewStruct(obj)
	if err != nil {
		return nil
	}

	return result
}

// GrpcStructToJsonObj — google.protobuf.Struct → JSON-объект. nil → nil.
func GrpcStructToJsonObj(v *structpb.Struct) []byte {
	if v == nil {
		return nil
	}

	result, err := json.Marshal(v.AsMap())
	if err != nil {
		return nil
	}

	return result
}
