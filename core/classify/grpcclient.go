package classify

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	internalv1 "github.com/MyauDev/vekst/core/gen/vekstinternal/v1"
)

// GRPCClient calls the classifier service over native gRPC on the cluster's
// private network. It is the only implementation of Classifier that talks to
// anything.
type GRPCClient struct {
	conn   *grpc.ClientConn
	client internalv1.ClassifierServiceClient
}

// Dial connects to the classifier. The connection is lazy: gRPC does not
// establish it until the first call, so a classifier that is not yet running
// does not prevent core from starting.
//
// Transport credentials are insecure because this is a ClusterIP address on a
// private network, never routed through the Ingress (design D6). When this
// leaves the cluster, it needs TLS, and that is the provisioning change's job.
func Dial(target string) (*GRPCClient, error) {
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("classifier: dial %q: %w", target, err)
	}
	return &GRPCClient{conn: conn, client: internalv1.NewClassifierServiceClient(conn)}, nil
}

// Version implements Classifier.
func (c *GRPCClient) Version(ctx context.Context) (VersionInfo, error) {
	resp, err := c.client.Version(ctx, &internalv1.VersionRequest{})
	if err != nil {
		return VersionInfo{}, fmt.Errorf("classifier: version: %w", err)
	}
	return VersionInfo{EngineVersion: resp.GetEngineVersion(), BuiltAt: resp.GetBuiltAt()}, nil
}

// Close releases the connection.
func (c *GRPCClient) Close() error { return c.conn.Close() }

// Unavailable is a Classifier that always fails. It is what core uses when no
// classifier address is configured, so that the "classifier is unreachable"
// path is the same code in development as in production rather than a nil check
// that only the tests ever exercise.
type Unavailable struct{}

// Version implements Classifier.
func (Unavailable) Version(context.Context) (VersionInfo, error) {
	return VersionInfo{}, fmt.Errorf("classifier: not configured")
}
