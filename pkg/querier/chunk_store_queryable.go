package querier

import (
	"context"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"

	"github.com/weaveworks/cortex/pkg/chunk"
)

type chunkIteratorFunc func(chunks []chunk.Chunk, from, through model.Time) storage.SeriesIterator

func newChunkStoreQueryable(store ChunkStore, chunkIteratorFunc chunkIteratorFunc) storage.Queryable {
	return storage.QueryableFunc(func(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
		return &chunkStoreQuerier{
			store:             store,
			chunkIteratorFunc: chunkIteratorFunc,
			ctx:               ctx,
			mint:              mint,
			maxt:              maxt,
		}, nil
	})
}

type chunkStoreQuerier struct {
	store             ChunkStore
	chunkIteratorFunc chunkIteratorFunc
	ctx               context.Context
	mint, maxt        int64
}

func (q *chunkStoreQuerier) Select(_ *storage.SelectParams, matchers ...*labels.Matcher) (storage.SeriesSet, error) {
	chunks, err := q.store.Get(q.ctx, model.Time(q.mint), model.Time(q.maxt), matchers...)
	if err != nil {
		return nil, promql.ErrStorage(err)
	}

	return q.partitionChunks(chunks), nil
}

func (q *chunkStoreQuerier) partitionChunks(chunks []chunk.Chunk) storage.SeriesSet {
	chunksBySeries := map[model.Fingerprint][]chunk.Chunk{}
	for _, c := range chunks {
		fp := c.Metric.Fingerprint()
		chunksBySeries[fp] = append(chunksBySeries[fp], c)
	}

	series := make([]storage.Series, 0, len(chunksBySeries))
	for i := range chunksBySeries {
		series = append(series, &chunkSeries{
			labels:            metricToLabels(chunksBySeries[i][0].Metric),
			chunks:            chunksBySeries[i],
			chunkIteratorFunc: q.chunkIteratorFunc,
			mint:              q.mint,
			maxt:              q.maxt,
		})
	}

	return newConcreteSeriesSet(series)
}

func (q *chunkStoreQuerier) LabelValues(name string) ([]string, error) {
	return nil, nil
}

func (q *chunkStoreQuerier) Close() error {
	return nil
}

type chunkSeries struct {
	labels            labels.Labels
	chunks            []chunk.Chunk
	chunkIteratorFunc chunkIteratorFunc
	mint, maxt        int64
}

func (s *chunkSeries) Labels() labels.Labels {
	return s.labels
}

// Iterator returns a new iterator of the data of the series.
func (s *chunkSeries) Iterator() storage.SeriesIterator {
	return s.chunkIteratorFunc(s.chunks, model.Time(s.mint), model.Time(s.maxt))
}
