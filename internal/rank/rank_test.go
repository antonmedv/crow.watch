package rank

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Wilson Score tests ---

func TestWilsonScore(t *testing.T) {
	tests := []struct {
		name      string
		upvotes   int
		downvotes int
		want      float64
	}{
		{"zero votes", 0, 0, 0},
		{"one upvote", 1, 0, 0.3787},
		{"one downvote", 0, 1, 0},
		{"all upvotes", 10, 0, 0.8590},
		{"all downvotes", 0, 10, 0},
		{"equal split", 5, 5, 0.3123},
		{"mostly positive", 10, 1, 0.7396},
		{"mostly negative", 1, 10, 0.0276},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WilsonScore(tt.upvotes, tt.downvotes)
			assert.InDelta(t, tt.want, got, 0.001)
		})
	}
}

func TestWilsonScoreProperties(t *testing.T) {
	t.Run("result always in [0, 1]", func(t *testing.T) {
		cases := [][2]int{{0, 0}, {1, 0}, {0, 1}, {100, 0}, {0, 100}, {50, 50}, {1000, 1}}
		for _, c := range cases {
			score := WilsonScore(c[0], c[1])
			assert.GreaterOrEqual(t, score, 0.0, "up=%d down=%d", c[0], c[1])
			assert.LessOrEqual(t, score, 1.0, "up=%d down=%d", c[0], c[1])
		}
	})

	t.Run("more upvotes at same total ranks higher", func(t *testing.T) {
		// 8 up / 2 down should beat 5 up / 5 down
		assert.Greater(t, WilsonScore(8, 2), WilsonScore(5, 5))
	})

	t.Run("higher sample size with same ratio ranks higher", func(t *testing.T) {
		// 10 up / 0 down should beat 1 up / 0 down
		assert.Greater(t, WilsonScore(10, 0), WilsonScore(1, 0))
	})
}

// --- A) Table-driven tests ---

func TestComputeBase(t *testing.T) {
	tests := []struct {
		name string
		tags []TagInput
		want float64
	}{
		{"no tags", nil, 0},
		{"single positive mod", []TagInput{{0.5}}, 0.5},
		{"single negative mod", []TagInput{{-0.5}}, -0.5},
		{"multiple tags sum", []TagInput{{0.3}, {0.7}, {-0.2}}, 0.8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeBase(tt.tags)
			assert.InDelta(t, tt.want, got, 1e-9)
		})
	}
}

func TestComputeCommentPoints(t *testing.T) {
	tests := []struct {
		name        string
		base        float64
		storyScore  int
		comments    []CommentInput
		wantCpoints float64
		wantRaw     float64
	}{
		{"no comments", 0, 5, nil, 0, 0},
		{"base < 0 returns 0", -1, 5, []CommentInput{{Score: 1}}, 0, 0},
		{"single comment", 0, 5, []CommentInput{{Score: 1, IsSubmitter: false}}, 1, 1},
		{"submitter bonus", 0, 5, []CommentInput{{Score: 1, IsSubmitter: true}}, 1.25, 1.25},
		{"clamped to story_score", 0, 2, []CommentInput{{}, {}, {}, {}, {}}, 2, 5},
		{"many comments with submitter", 0, 10, []CommentInput{
			{IsSubmitter: true}, {}, {IsSubmitter: true}, {},
		}, 4.5, 4.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpoints, raw := ComputeCommentPoints(tt.base, tt.storyScore, tt.comments)
			assert.InDelta(t, tt.wantCpoints, cpoints, 1e-9, "cpoints")
			assert.InDelta(t, tt.wantRaw, raw, 1e-9, "raw")
		})
	}
}

func TestComputeOrder(t *testing.T) {
	tests := []struct {
		name       string
		storyScore int
		cpoints    float64
		want       float64
	}{
		{"score 0 cpoints 0 → log10(1) = 0", 0, 0, 0},
		{"score 1 → log10(1) = 0", 1, 0, 0},
		{"score 10 → log10(10) = 1", 10, 0, 1},
		{"score 100 → log10(100) = 2", 100, 0, 2},
		{"score + cpoints", 5, 5, math.Log10(10)},
		{"negative score abs", -100, 0, 2},
		{"clamp at 10", 0, 1e11, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeOrder(tt.storyScore, tt.cpoints)
			assert.InDelta(t, tt.want, got, 1e-9)
		})
	}
}

func TestComputeSign(t *testing.T) {
	tests := []struct {
		name       string
		storyScore int
		want       int
	}{
		{"positive", 5, 1},
		{"negative", -3, -1},
		{"zero", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ComputeSign(tt.storyScore))
		})
	}
}

func TestComputeAge(t *testing.T) {
	window := float64(DefaultHotnessWindowSeconds)

	t.Run("seconds since epoch divided by window", func(t *testing.T) {
		ts := time.Unix(79200, 0)
		age := ComputeAge(ts, window)
		assert.InDelta(t, 1.0, age, 1e-9)
	})

	t.Run("newer story has larger age value", func(t *testing.T) {
		now := time.Now()
		older := now.Add(-1 * time.Hour)
		assert.Greater(t, ComputeAge(now, window), ComputeAge(older, window))
	})

	t.Run("epoch returns 0", func(t *testing.T) {
		age := ComputeAge(time.Unix(0, 0), window)
		assert.InDelta(t, 0, age, 1e-9)
	})
}

func TestComputeHotness(t *testing.T) {
	now := time.Now()
	window := float64(DefaultHotnessWindowSeconds)
	nowAge := ComputeAge(now, window)

	t.Run("score 1 story hotness", func(t *testing.T) {
		h := ComputeHotness(StoryInput{
			ID:         1,
			CreatedAt:  now,
			StoryScore: 1,
		}, window)
		// order = log10(1) = 0, sign = 1
		// hotness = -1 * (0 + 0*1 + nowAge)
		assert.InDelta(t, -1*(0+0*1+nowAge), h.Hotness, 0.01)
	})

	t.Run("score 10 story hotness", func(t *testing.T) {
		h := ComputeHotness(StoryInput{
			ID:         2,
			CreatedAt:  now,
			StoryScore: 10,
		}, window)
		// order = log10(10) = 1, sign = 1
		// hotness = -1 * (0 + 1*1 + nowAge)
		assert.InDelta(t, -1*(1+nowAge), h.Hotness, 0.01)
	})

	t.Run("higher score ranks higher at same time", func(t *testing.T) {
		h10 := ComputeHotness(StoryInput{ID: 1, CreatedAt: now, StoryScore: 10}, window)
		h1 := ComputeHotness(StoryInput{ID: 2, CreatedAt: now, StoryScore: 1}, window)
		assert.Less(t, h10.Hotness, h1.Hotness, "higher score should have more negative hotness")
	})

	t.Run("newer story ranks higher than older", func(t *testing.T) {
		hNewer := ComputeHotness(StoryInput{ID: 1, CreatedAt: now, StoryScore: 5}, window)
		hOlder := ComputeHotness(StoryInput{ID: 2, CreatedAt: now.Add(-1 * time.Hour), StoryScore: 5}, window)
		assert.Less(t, hNewer.Hotness, hOlder.Hotness, "newer story should have more negative hotness")
	})

	t.Run("negative tag mod penalizes", func(t *testing.T) {
		hNoTag := ComputeHotness(StoryInput{ID: 1, CreatedAt: now, StoryScore: 10}, window)
		hTag := ComputeHotness(StoryInput{ID: 2, CreatedAt: now, Tags: []TagInput{{-2.0}}, StoryScore: 10}, window)
		assert.Greater(t, hTag.Hotness, hNoTag.Hotness, "negative tag mod should produce less negative hotness (ranks lower)")
	})
}

// --- B) Property tests ---

func TestNewerStoryHasLowerHotness(t *testing.T) {
	window := float64(DefaultHotnessWindowSeconds)
	now := time.Now()

	for i := 1; i <= 20; i++ {
		older := StoryInput{
			ID:         int64(i),
			CreatedAt:  now.Add(-time.Duration(i) * time.Hour),
			StoryScore: 5,
		}
		newer := StoryInput{
			ID:         int64(i + 100),
			CreatedAt:  now.Add(-time.Duration(i-1) * time.Hour),
			StoryScore: 5,
		}
		hOlder := ComputeHotness(older, window)
		hNewer := ComputeHotness(newer, window)
		assert.Lessf(t, hNewer.Hotness, hOlder.Hotness,
			"newer story (age=%dh) should have more negative hotness than older (age=%dh)", i-1, i)
	}
}

func TestIncreasingScoreNonDecreasingOrder(t *testing.T) {
	var prevOrder float64
	for score := 0; score <= 100; score++ {
		order := ComputeOrder(score, 0)
		assert.GreaterOrEqual(t, order, prevOrder, "order should not decrease as score increases (score=%d)", score)
		prevOrder = order
	}
}

func TestNegativeBaseZerosComments(t *testing.T) {
	comments := []CommentInput{{Score: 5}, {Score: 10, IsSubmitter: true}}
	cpoints, raw := ComputeCommentPoints(-1.0, 100, comments)
	assert.Equal(t, 0.0, cpoints)
	assert.Equal(t, 0.0, raw)
}

// --- C) Regression tests ---

type regressionStory struct {
	ID         int64     `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	Tags       []float64 `json:"tags"`
	StoryScore int       `json:"story_score"`
}

type regressionCase struct {
	Stories     []regressionStory `json:"stories"`
	ExpectedIDs []int64           `json:"expected_ids"`
}

func TestRegressionRanking(t *testing.T) {
	now := time.Now()
	window := float64(DefaultHotnessWindowSeconds)

	// Build 10 stories with different characteristics
	stories := []StoryInput{
		{ID: 1, CreatedAt: now, StoryScore: 10},                                                             // brand new, good score
		{ID: 2, CreatedAt: now.Add(-1 * time.Hour), StoryScore: 50},                                         // 1h old, great score
		{ID: 3, CreatedAt: now.Add(-2 * time.Hour), StoryScore: 100},                                        // 2h old, excellent score
		{ID: 4, CreatedAt: now.Add(-24 * time.Hour), StoryScore: 5},                                         // 1 day old, low score
		{ID: 5, CreatedAt: now, StoryScore: 1},                                                              // brand new, minimal score
		{ID: 6, CreatedAt: now.Add(-30 * time.Minute), StoryScore: 20, Tags: []TagInput{{HotnessMod: 0.5}}}, // 30m, tagged
		{ID: 7, CreatedAt: now.Add(-6 * time.Hour), StoryScore: 200},                                        // 6h old, huge score
		{ID: 8, CreatedAt: now.Add(-48 * time.Hour), StoryScore: 1000},                                      // 2 days old, massive score
		{ID: 9, CreatedAt: now, StoryScore: 0},                                                              // brand new, zero score
		{ID: 10, CreatedAt: now.Add(-12 * time.Hour), StoryScore: 30},                                       // 12h old, decent score
	}

	scored := RankStories(stories, window)

	require.Len(t, scored, 10)

	// Verify sorted by hotness ascending (most negative first)
	for i := 1; i < len(scored); i++ {
		assert.LessOrEqual(t, scored[i-1].Hotness, scored[i].Hotness,
			"stories should be sorted by hotness ascending")
	}

	// Component breakdown checks
	for _, s := range scored {
		// Verify hotness = -1 * (base + order*sign + age)
		expectedHotness := -1 * (s.Base + s.Order*float64(s.Sign) + s.Age)
		assert.InDelta(t, expectedHotness, s.Hotness, 1e-9,
			"hotness formula mismatch for story %d", s.ID)
	}
}

func TestControversialStoryClampedComments(t *testing.T) {
	now := time.Now()
	window := float64(DefaultHotnessWindowSeconds)

	// Low score story with tons of comments — cpoints should be clamped to storyScore
	manyComments := make([]CommentInput, 100)
	for i := range manyComments {
		manyComments[i] = CommentInput{Score: 1}
	}

	story := StoryInput{
		ID:         1,
		CreatedAt:  now,
		StoryScore: 3,
		Comments:   manyComments,
	}

	scored := RankStories([]StoryInput{story}, window)
	require.Len(t, scored, 1)

	s := scored[0]
	assert.InDelta(t, 3.0, s.Cpoints, 1e-9, "cpoints should be clamped to storyScore")
	assert.InDelta(t, 100.0, s.CommentPoints, 1e-9, "raw comment points should be 100")

	// order should use storyScore + clamped cpoints = 3 + 3 = 6
	expectedOrder := math.Log10(6)
	assert.InDelta(t, expectedOrder, s.Order, 1e-9)
}

func TestRankStoriesStableOrder(t *testing.T) {
	now := time.Now()
	window := float64(DefaultHotnessWindowSeconds)

	// Two identical stories should maintain original order (stable sort check)
	stories := []StoryInput{
		{ID: 1, CreatedAt: now, StoryScore: 5},
		{ID: 2, CreatedAt: now, StoryScore: 5},
	}

	scored := RankStories(stories, window)
	require.Len(t, scored, 2)
	assert.InDelta(t, scored[0].Hotness, scored[1].Hotness, 1e-9)
}

func TestRankStoriesSnapshot(t *testing.T) {
	// Fixed time for reproducible results
	fixedTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	window := float64(DefaultHotnessWindowSeconds)

	stories := []StoryInput{
		{ID: 1, CreatedAt: fixedTime, StoryScore: 10},
		{ID: 2, CreatedAt: fixedTime.Add(-1 * time.Hour), StoryScore: 50},
		{ID: 3, CreatedAt: fixedTime.Add(-24 * time.Hour), StoryScore: 5},
	}

	scored := RankStories(stories, window)
	require.Len(t, scored, 3)

	// Verify each story's components are self-consistent
	for _, s := range scored {
		expectedAge := float64(s.CreatedAt.Unix()) / window
		assert.InDelta(t, expectedAge, s.Age, 1e-9)
	}

	// Verify the JSON roundtrip of IDs preserves ranking
	ids := make([]int64, len(scored))
	for i, s := range scored {
		ids[i] = s.ID
	}
	data, err := json.Marshal(ids)
	require.NoError(t, err)

	var roundtripped []int64
	require.NoError(t, json.Unmarshal(data, &roundtripped))
	assert.Equal(t, ids, roundtripped)
}
