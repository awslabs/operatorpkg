package middleware

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/smithy-go/middleware"
	"github.com/awslabs/operatorpkg/serrors"
	"github.com/samber/lo"
)

const AWSRequestIDLogKey = "aws-request-id"

var RequestIDErrorHandler = func(stack *middleware.Stack) error {
	return stack.Deserialize.Add(middleware.DeserializeMiddlewareFunc("RequestIDErrorHandler", func(ctx context.Context, in middleware.DeserializeInput, next middleware.DeserializeHandler) (middleware.DeserializeOutput, middleware.Metadata, error) {
		out, metadata, err := next.HandleDeserialize(ctx, in)
		if err != nil {
			if v, ok := lo.ErrorsAs[*http.ResponseError](err); ok {
				err = serrors.Wrap(err, AWSRequestIDLogKey, v.ServiceRequestID())
			}
		}
		return out, metadata, err
	}), middleware.Before)
}
