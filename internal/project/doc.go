// Package project owns UltraPlan project planning roots.
//
// A project is a directory under projects/<name> containing editable planning
// documents, a roadmap, a project-index.md catalog, and sprint directories.
// This package discovers those roots, reads their project-owned files, parses
// the project index as a catalog, and validates catalog references.
//
// Project indexes are catalogs only. They are not sprint plans, workflow state,
// runtime prompts, or study reports, and this package intentionally does not
// consume study services or runtime execution packages.
package project
