package blob

import "github.com/google/uuid"

// Key builds the object key for a batch's upload: org/<org_id>/batch/<batch_id>.
//
// Built from identifiers only, never from the file name a browser sent. A
// user-supplied name is a path traversal and a collision at once; an
// identifier this package did not choose is neither. The org segment is not
// the tenant boundary -- the database policy is -- but it does mean an
// operator looking at the bucket can tell whose object they are looking at
// without opening it.
func Key(orgID, batchID uuid.UUID) string {
	return "org/" + orgID.String() + "/batch/" + batchID.String()
}
