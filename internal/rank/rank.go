package rank

import (
	"math"
	"sort"
	"time"
)

const DefaultHotnessWindowSeconds = 22 * 60 * 60 // 22 hours

// WilsonScore computes the lower bound of the Wilson score confidence interval
// for a Bernoulli parameter (80% confidence, z = 1.281). This is used for ranking
// comments — higher scores indicate higher confidence of a positive rating.
func WilsonScore(upvotes, downvotes int) float64 {
	n := float64(upvotes + downvotes)
	if n == 0 {
		return 0
	}
	const z = 1.281
	phat := float64(upvotes) / n
	return (phat + z*z/(2*n) - z*math.Sqrt((phat*(1-phat)+z*z/(4*n))/n)) / (1 + z*z/n)
}

type TagInput struct {
	HotnessMod float64
}

type CommentInput struct {
	Score       int
	IsSubmitter bool
}

type StoryInput struct {
	ID         int64
	CreatedAt  time.Time
	Tags       []TagInput
	StoryScore int
	Comments   []CommentInput
}

type ScoredStory struct {
	StoryInput
	Hotness       float64
	Base          float64
	Order         float64
	Sign          int
	Age           float64
	CommentPoints float64
	Cpoints       float64
}

// ComputeBase calculates the base penalty from tags.
// Each tag's hotness_mod is summed.
func ComputeBase(tags []TagInput) float64 {
	var base float64
	for _, t := range tags {
		base += t.HotnessMod
	}
	return base
}

// ComputeCommentPoints calculates the comment contribution.
// If base < 0 (heavy penalty), comment points are 0.
// Raw comment points: each comment is 1 + (bonus 0.25 if submitter).
// Final cpoints is clamped to storyScore.
func ComputeCommentPoints(base float64, storyScore int, comments []CommentInput) (cpoints, rawCommentPoints float64) {
	if base < 0 {
		return 0, 0
	}
	for _, c := range comments {
		rawCommentPoints += 1.0
		if c.IsSubmitter {
			rawCommentPoints += 0.25
		}
	}
	cpoints = math.Min(rawCommentPoints, float64(storyScore))
	return cpoints, rawCommentPoints
}

// ComputeOrder computes log10(max(abs(storyScore + cpoints), 1)).
// Clamped to a maximum of 10.
func ComputeOrder(storyScore int, cpoints float64) float64 {
	v := math.Abs(float64(storyScore) + cpoints)
	if v < 1 {
		v = 1
	}
	return math.Min(math.Log10(v), 10)
}

// ComputeSign returns 1 if storyScore > 0, -1 if < 0, 0 otherwise.
func ComputeSign(storyScore int) int {
	if storyScore > 0 {
		return 1
	}
	if storyScore < 0 {
		return -1
	}
	return 0
}

// ComputeAge returns the time component: seconds since epoch / windowSeconds.
// Newer stories have larger values, which when multiplied by -1 in the hotness
// formula, produces more negative hotness (= ranks higher in ascending sort).
func ComputeAge(createdAt time.Time, windowSeconds float64) float64 {
	return float64(createdAt.Unix()) / windowSeconds
}

// ComputeHotness calculates the full hotness score for a story.
// hotness = -1 * (base + order * sign + age)
// Lower (more negative) hotness values rank higher.
func ComputeHotness(story StoryInput, windowSeconds float64) ScoredStory {
	base := ComputeBase(story.Tags)
	cpoints, rawCommentPoints := ComputeCommentPoints(base, story.StoryScore, story.Comments)
	order := ComputeOrder(story.StoryScore, cpoints)
	sign := ComputeSign(story.StoryScore)
	age := ComputeAge(story.CreatedAt, windowSeconds)
	hotness := -1 * (base + order*float64(sign) + age)

	return ScoredStory{
		StoryInput:    story,
		Hotness:       hotness,
		Base:          base,
		Order:         order,
		Sign:          sign,
		Age:           age,
		CommentPoints: rawCommentPoints,
		Cpoints:       cpoints,
	}
}

// RankStories computes hotness for each story and returns them sorted (hottest first = most negative hotness).
func RankStories(stories []StoryInput, windowSeconds float64) []ScoredStory {
	scored := make([]ScoredStory, len(stories))
	for i, s := range stories {
		scored[i] = ComputeHotness(s, windowSeconds)
	}
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Hotness < scored[j].Hotness
	})
	return scored
}
