package response

import (
	"net/http"
	"strings"

	apperrors "github.com/CaixyPromise/seven-framework/seven-framework-server/internal/common/errors"
	jsoncompat "github.com/CaixyPromise/seven-framework/seven-framework-server/internal/common/json"
	"github.com/CaixyPromise/seven-framework/seven-framework-server/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
)

type Result struct {
	Code    int    `json:"code"`
	Data    any    `json:"data"`
	Message string `json:"message"`
	TraceID string `json:"traceId,omitempty"`
}

func Success(c *app.RequestContext, data any) {
	write(c, http.StatusOK, Result{
		Code:    apperrors.CodeSuccess,
		Data:    jsoncompat.NormalizeForJSON(data),
		Message: "ok",
		TraceID: xcontext.TraceID(c),
	})
}

func Error(c *app.RequestContext, err error) {
	appErr := apperrors.From(err)
	xcontext.SetResponseError(c, appErr.Code(), appErr.Message())
	if appErr.Code() == apperrors.CodeSystemError {
		if cause := strings.TrimSpace(err.Error()); cause != "" && cause != appErr.Message() {
			xcontext.SetResponseErrorCause(c, cause)
		}
	}
	write(c, apperrors.HTTPStatus(appErr), Result{
		Code:    appErr.Code(),
		Data:    jsoncompat.NormalizeForJSON(appErr.Details()),
		Message: appErr.Message(),
		TraceID: xcontext.TraceID(c),
	})
}

func write(c *app.RequestContext, status int, body Result) {
	c.JSON(status, body)
}
