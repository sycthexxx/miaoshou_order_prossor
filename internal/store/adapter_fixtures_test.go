package store

// Existing integration fixtures exercise adapters and persistence together.
// Production store code only depends on domain contracts.
import (
	"database/sql"
	"order_processor/internal/adapters/miaoshou/web"
	"order_processor/internal/domain/procurement"
)

const (
	listEndpoint        = web.ListEndpoint
	translationEndpoint = web.TranslationEndpoint
	productEndpoint     = web.ProductEndpoint
	outboundEndpoint    = web.OutboundEndpoint
)

func Replay(db *sql.DB, dir string) (int, error) { return ReplayArchives(db, dir, web.Decoder{}) }

func procurementSummary(rows []object, gifts bool) object { return procurement.Summarize(rows, gifts) }
