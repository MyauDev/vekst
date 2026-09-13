package classify

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	internalv1 "github.com/MyauDev/vekst/core/gen/vekstinternal/v1"
	typev1 "github.com/MyauDev/vekst/core/gen/vekstype/v1"
	"github.com/MyauDev/vekst/core/internal/money"
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

// Classify implements Classifier.
//
// The translation below is the only place in core that knows a batch is
// protobuf. Money crosses it as money.Money in both directions: never a
// float, and never an amount without the code needed to read it.
func (c *GRPCClient) Classify(ctx context.Context, req BatchRequest) (BatchResponse, error) {
	resp, err := c.client.ClassifyBatch(ctx, toProto(req))
	if err != nil {
		return BatchResponse{}, fmt.Errorf("classifier: classify batch %q: %w", req.RequestID, err)
	}
	out := BatchResponse{
		EngineVersion:  resp.GetEngineVersion(),
		RulesetVersion: resp.GetRulesetVersion(),
		Proposals:      make([]Proposal, 0, len(resp.GetProposals())),
	}
	for _, p := range resp.GetProposals() {
		out.Proposals = append(out.Proposals, Proposal{
			TransactionID:       p.GetTransactionId(),
			CategoryCode:        p.GetCategoryCode(),
			EngineLayer:         p.GetEngineLayer(),
			Confidence:          p.GetConfidence(),
			Evidence:            p.GetEvidence(),
			MatchedRulePriority: p.GetMatchedRulePriority(),
		})
	}
	return out, nil
}

func toProto(req BatchRequest) *internalv1.ClassifyBatchRequest {
	out := &internalv1.ClassifyBatchRequest{
		RequestId:        req.RequestID,
		TaxonomyVersion:  req.TaxonomyVersion,
		RulesetVersion:   req.RulesetVersion,
		NormalizeVersion: req.NormalizeVersion,
		Threshold:        req.Threshold,
		Categories:       make([]*internalv1.Category, 0, len(req.Categories)),
		Rules:            make([]*internalv1.Rule, 0, len(req.Rules)),
		Vendors:          make([]*internalv1.VendorMemory, 0, len(req.Vendors)),
		Txns:             make([]*internalv1.TxnForClassify, 0, len(req.Txns)),
	}
	for _, c := range req.Categories {
		out.Categories = append(out.Categories, &internalv1.Category{
			Code:               c.Code,
			Name:               c.Name,
			RequiresAllocation: c.RequiresAllocation,
		})
	}
	for _, r := range req.Rules {
		rule := &internalv1.Rule{
			Priority:     r.Priority,
			CategoryCode: r.CategoryCode,
			Scope:        r.Scope,
			SourceKind:   r.SourceKind,
			All:          make([]*internalv1.Condition, 0, len(r.All)),
		}
		for _, cond := range r.All {
			rule.All = append(rule.All, &internalv1.Condition{
				Field:       cond.Field,
				Op:          cond.Op,
				Value:       cond.Value,
				AmountValue: toMoney(cond.AmountValue),
			})
		}
		out.Rules = append(out.Rules, rule)
	}
	for _, v := range req.Vendors {
		out.Vendors = append(out.Vendors, &internalv1.VendorMemory{
			Key:          v.Key,
			CategoryCode: v.CategoryCode,
			DisplayName:  v.DisplayName,
		})
	}
	for _, t := range req.Txns {
		out.Txns = append(out.Txns, &internalv1.TxnForClassify{
			TransactionId:   t.TransactionID,
			SourceKind:      t.SourceKind,
			DescriptionNorm: t.DescriptionNorm,
			CounterpartyKey: t.CounterpartyKey,
			Direction:       t.Direction,
			Amount:          toMoney(t.Amount),
			RegulatedCode:   t.RegulatedCode,
			AccountId:       t.AccountID,
		})
	}
	return out
}

// toMoney leaves an absent amount absent rather than sending a zero in an
// empty currency. "No amount" and "zero of some currency we did not name" are
// different claims, and only one of them is true of a rule that does not match
// on an amount at all.
func toMoney(m money.Money) *typev1.Money {
	if m.CurrencyCode == "" && m.MinorUnits == 0 {
		return nil
	}
	return &typev1.Money{CurrencyCode: m.CurrencyCode, MinorUnits: m.MinorUnits}
}

// Classify implements Classifier.
func (Unavailable) Classify(context.Context, BatchRequest) (BatchResponse, error) {
	return BatchResponse{}, fmt.Errorf("classifier: not configured")
}
