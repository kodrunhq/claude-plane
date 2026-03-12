// Package grpc provides the gRPC server for claude-plane,
// including mTLS authentication and agent connection management.
package grpc

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// machineIDKeyType is an unexported type for the context key to prevent collisions.
type machineIDKeyType struct{}

// machineIDKey is the context key for the machine-id extracted from client certs.
var machineIDKey = machineIDKeyType{}

// MachineIDFromContext retrieves the machine-id from the context.
// Returns an error if the machine-id is not present.
func MachineIDFromContext(ctx context.Context) (string, error) {
	v, ok := ctx.Value(machineIDKey).(string)
	if !ok || v == "" {
		return "", status.Error(codes.Unauthenticated, "machine-id not found in context")
	}
	return v, nil
}

// extractMachineID extracts the machine-id from the client certificate's CN
// in the gRPC peer context. The CN must have the format "agent-{machine-id}".
func extractMachineID(ctx context.Context) (string, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "no peer info in context")
	}

	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "no TLS info in peer")
	}

	if len(tlsInfo.State.PeerCertificates) == 0 {
		return "", status.Error(codes.Unauthenticated, "no client certificate presented")
	}

	cn := tlsInfo.State.PeerCertificates[0].Subject.CommonName
	if !strings.HasPrefix(cn, "agent-") {
		return "", status.Errorf(codes.PermissionDenied, "invalid agent certificate CN: %q (must start with \"agent-\")", cn)
	}

	machineID := strings.TrimPrefix(cn, "agent-")
	if machineID == "" {
		return "", status.Error(codes.PermissionDenied, "empty machine-id in certificate CN")
	}

	return machineID, nil
}

// MachineAuthUnaryInterceptor returns a gRPC unary server interceptor that
// extracts the machine-id from the client certificate and attaches it to the context.
func MachineAuthUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		machineID, err := extractMachineID(ctx)
		if err != nil {
			return nil, err
		}
		ctx = context.WithValue(ctx, machineIDKey, machineID)
		return handler(ctx, req)
	}
}

// MachineAuthStreamInterceptor returns a gRPC stream server interceptor that
// extracts the machine-id from the client certificate and attaches it to the stream context.
func MachineAuthStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		machineID, err := extractMachineID(ss.Context())
		if err != nil {
			return err
		}
		ctx := context.WithValue(ss.Context(), machineIDKey, machineID)
		wrapped := &wrappedServerStream{ServerStream: ss, ctx: ctx}
		return handler(srv, wrapped)
	}
}

// wrappedServerStream wraps a grpc.ServerStream to override the Context.
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the enriched context with the machine-id.
func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}
