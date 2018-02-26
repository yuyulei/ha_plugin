package ha

import (
	"net/http"

	"qiniu.com/apps/rpc"
)

// 本身这里可以有自己随意的特有 error , 但为了跟现有的 sdk 客户端相匹配, 故沿用 HttpError
var (
	ErrFailToGetValidLeaderInfo     = rpc.NewHTTPError(http.StatusInternalServerError, "HA ERROR", "fail to get leader info, maybe ha is setting up.")
	ErrFailToConnectToLeader        = rpc.NewHTTPError(http.StatusInternalServerError, "HA ERROR", "fail to connect leader node, maybe bad network.")
	ErrFailToReadRespBodyFromLeader = rpc.NewHTTPError(http.StatusInternalServerError, "HA ERROR", "fail to read body of response from leader.")
)
