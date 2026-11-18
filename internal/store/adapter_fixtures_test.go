package store

// Existing integration fixtures exercise adapters and persistence together.
// Production store code only depends on domain contracts.
import (
	"context"
	"database/sql"
	miaoshouopenapi "order_processor/internal/adapters/miaoshou/openapi"
	"order_processor/internal/adapters/miaoshou/web"
	"order_processor/internal/domain/orders"
	"order_processor/internal/domain/procurement"
)

const (
	listEndpoint        = web.ListEndpoint
	translationEndpoint = web.TranslationEndpoint
	productEndpoint     = web.ProductEndpoint
	outboundEndpoint    = web.OutboundEndpoint
)

func Replay(db *sql.DB, dir string) (int, error) { return ReplayArchives(db, dir, web.Decoder{}) }
func importPendingFixture(ctx context.Context, db *sql.DB, account, at, source string, snapshot miaoshouopenapi.PendingSnapshot) (orders.ImportResult, error) {
	batch, err := miaoshouopenapi.PendingBatch(snapshot)
	if err != nil {
		return orders.ImportResult{}, err
	}
	batch.CapturedAt = at
	batch.Archive = source
	return ImportBatch(ctx, db, account, batch)
}

func procurementSummary(rows []object, gifts bool) object { return procurement.Summarize(rows, gifts) }
