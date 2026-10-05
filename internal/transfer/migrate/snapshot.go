package migrate

import (
	"maps"
	"slices"
)

func Snapshot(job *Job) *Job {
	if job == nil {
		return nil
	}
	snapshot := *job
	snapshot.FileIDs = slices.Clone(job.FileIDs)
	snapshot.CompletedFileIDs = slices.Clone(job.CompletedFileIDs)
	snapshot.CopiedFileIDs = slices.Clone(job.CopiedFileIDs)
	snapshot.Items = maps.Clone(job.Items)
	snapshot.TargetDirectoryIDs = maps.Clone(job.TargetDirectoryIDs)
	return &snapshot
}
