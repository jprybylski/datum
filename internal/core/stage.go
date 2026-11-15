package core

import (
	"fmt"
	"path/filepath"
	"sync"
)

// stagePlan maps each dataset of a staged fetch (#38) to its real target.
type stagePlan struct {
	targets []stagedTarget // indexed like Config.Datasets

	// removed collects, per real target folder, the files datasets fetched so far in this run
	// no longer write, so a later dataset sharing that folder hashes it as an in-place fetch
	// would see it at that point.
	mu      sync.Mutex
	removed map[string]map[string]bool
}

// stagedTarget is where a dataset's target really lives, as opposed to the stage copy datum
// writes: rel is the target relative to the project, local is the real path.
type stagedTarget struct {
	rel   string
	local string
	plan  *stagePlan
}

// removeAndList records rels as removed from local and returns everything removed from it so
// far in this run.
func (p *stagePlan) removeAndList(local string, rels []string) map[string]bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := filepath.Clean(local)
	set := p.removed[key]
	if set == nil {
		set = map[string]bool{}
		p.removed[key] = set
	}
	for _, rel := range rels {
		set[rel] = true
	}
	all := make(map[string]bool, len(set))
	for rel := range set {
		all[rel] = true
	}
	return all
}

// planStage points every dataset's target into opts.StageDir, keeping the project-relative
// layout, and remembers the real targets. Selected datasets must have relative targets without
// ".."; others keep theirs if they can't be staged, since they won't be fetched.
//
// Every dataset is pointed into the stage, not only the selected ones, so that datasets sharing
// a target folder still see each other there (claimedPaths compares targets).
func planStage(cfg *Config, info *configInfo, opts Options, which map[string]bool) (*stagePlan, error) {
	stageAbs, err := filepath.Abs(opts.StageDir)
	if err != nil {
		return nil, fmt.Errorf("--stage-dir: %w", err)
	}
	plan := &stagePlan{targets: make([]stagedTarget, len(cfg.Datasets)), removed: map[string]map[string]bool{}}
	for i := range cfg.Datasets {
		ds := &cfg.Datasets[i]
		selected := len(which) == 0 || which[ds.ID]
		rel, err := cleanRelative(info.relTargets[i])
		if err != nil || info.unresolved[i] {
			if selected {
				return nil, fmt.Errorf("dataset %d (%s): target: a staged fetch needs a relative target without \"..\" (%v)", i, ds.ID, err)
			}
			continue
		}
		plan.targets[i] = stagedTarget{rel: rel, local: ds.Target, plan: plan}
		ds.Target = filepath.Join(stageAbs, rel)
	}
	return plan, nil
}
