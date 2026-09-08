# `ui/` — what both registers use

Primitives that carry no register of their own: `StateChip`, `Money`,
`EmptyState`, `Skeleton`, `DataTable`, `Kbd`.

A component belongs here only if both registers can use it. `PnlTable` does not
— it is Register B's and lives in `app/`, and it is deliberately separate from
`DataTable` because freezing a column and scrolling sideways is its whole job.

Nothing here may hardcode a spacing step that only one register can afford.
Take it as a prop.
