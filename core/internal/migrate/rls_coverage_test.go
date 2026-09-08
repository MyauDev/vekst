package migrate

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The row-level-security coverage checker (design D6).
//
// It asserts isolation, not the existence of a policy. "Has at least one row
// in pg_policies" is far too weak a gate: every one of these passes it and
// none of them isolates anything.
//
//	CREATE POLICY p ON t FOR ALL USING (true);                           -- no isolation
//	CREATE POLICY p ON t FOR ALL USING (created_by = app_current_org()); -- wrong column
//	CREATE POLICY p ON t FOR SELECT USING (org_id = app_current_org());  -- writes denied
//
// The checker is a plain function rather than a test so that TestTheCheckerCanFail
// can run it against deliberately broken schemas and assert on what it
// reports. A green test that cannot go red proves nothing, and the likeliest
// real failure is a policy that exists and does not isolate.

// exemptTables reads the checked-in allowlist. A line here is a security
// decision -- it says no tenant's data can ever land in that table -- and
// CODEOWNERS requires both reviewers for the file.
func exemptTables(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	exempt := map[string]bool{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		exempt[line] = true
	}
	return exempt, s.Err()
}

// tenantColumnFor names the column a table's policy must restrict.
//
// It is org_id everywhere except organizations, where it is the primary key:
// the organisation *is* the tenant and carries no org_id of its own. A checker
// that simply demanded an org_id column would fail on the first table this
// change creates, which is why the exception is written here rather than
// discovered later.
func tenantColumnFor(table string) string {
	if table == "organizations" {
		return "id"
	}
	return "org_id"
}

// checkRLSCoverage returns one finding per problem, empty when the schema is
// sound. Findings are strings because they are read by a human in CI output.
func checkRLSCoverage(db *sql.DB, allowlistPath string) ([]string, error) {
	exempt, err := exemptTables(allowlistPath)
	if err != nil {
		return nil, fmt.Errorf("reading the allowlist: %w", err)
	}

	// relkind matters, and enumerating only ordinary tables ('r') is the
	// mistake this query exists to avoid:
	//
	//   'r' ordinary table            -- the normal case
	//   'p' partitioned table         -- holds no rows itself, but its policy
	//                                    is what governs access through it, so
	//                                    skipping it leaves the policy unchecked
	//   'm' materialized view         -- CANNOT have row-level security at all,
	//                                    and 00001's ALTER DEFAULT PRIVILEGES
	//                                    grants vekst_app SELECT on one the
	//                                    moment it is created
	//   'f' foreign table             -- policies are not enforced remotely
	//
	// A partition ('r' with relispartition) is included deliberately rather
	// than skipped: Postgres applies the *parent's* policies only when a row
	// is reached through the parent. A direct `SELECT FROM child` sees only
	// the child's own policies, and a partition inherits neither RLS flags nor
	// policies from its parent. Verified against Postgres 16: with the tenant
	// context set to org A, selecting from the parent returned 0 rows and
	// selecting from the partition returned org B's row.
	rows, err := db.Query(`
		SELECT c.relname, c.relkind, c.relrowsecurity, c.relforcerowsecurity, c.relispartition
		  FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = 'public'
		   AND c.relkind IN ('r', 'p', 'm', 'f')
		 ORDER BY c.relname`)
	if err != nil {
		return nil, fmt.Errorf("listing relations: %w", err)
	}
	defer rows.Close()

	var findings []string
	type relation struct {
		name            string
		kind            string
		enabled, forced bool
		isPartition     bool
	}
	var relations []relation
	for rows.Next() {
		var r relation
		if err := rows.Scan(&r.name, &r.kind, &r.enabled, &r.forced, &r.isPartition); err != nil {
			return nil, err
		}
		relations = append(relations, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, r := range relations {
		if exempt[r.name] {
			continue
		}

		// A materialized view holds a frozen copy of whatever its defining
		// query returned, and no policy applies to reading it. There is no
		// way to make one safe over a tenant table, so it is a hard failure
		// rather than something to be given a policy.
		if r.kind == "m" {
			findings = append(findings, fmt.Sprintf(
				"%s is a materialized view: row-level security cannot be enabled on one, "+
					"and 00001's default privileges grant vekst_app SELECT on it. "+
					"Materialize into an ordinary table with a policy, or allowlist it "+
					"and justify why no tenant's data can reach it", r.name))
			continue
		}
		if r.kind == "f" {
			findings = append(findings, fmt.Sprintf(
				"%s is a foreign table: policies are not enforced on the remote side", r.name))
			continue
		}

		// A partition is checked before the generic flags so that it gets the
		// message that explains itself. Its parent having a policy is not
		// enough and reads as though it were: Postgres applies the parent's
		// policies only to rows reached *through* the parent, and a partition
		// inherits neither the RLS flags nor the policies. `SELECT FROM child`
		// sees the child's own policies, of which there are none by default.
		if r.isPartition && !(r.enabled && r.forced) {
			findings = append(findings, fmt.Sprintf(
				"%s is a partition and needs its own policy: a partition inherits neither "+
					"RLS flags nor policies, and a direct select on it bypasses the parent's", r.name))
			continue
		}

		if !r.enabled {
			findings = append(findings, fmt.Sprintf(
				"%s does not have row-level security enabled", r.name))
			continue
		}
		if !r.forced {
			// Distinct from "not enabled" and worth its own message: without
			// FORCE the table owner is exempt from its own policies, which
			// restores a bypass silently, in pg_class rather than anywhere a
			// reviewer looks.
			findings = append(findings, fmt.Sprintf(
				"%s has row-level security enabled but not FORCEd, so the table owner is exempt", r.name))
		}

		f, err := checkPolicies(db, r.name)
		if err != nil {
			return nil, err
		}
		findings = append(findings, f...)
	}

	inventory, err := checkExceptionInventories(db)
	if err != nil {
		return nil, err
	}
	findings = append(findings, inventory...)

	roles, err := checkRoleAttributes(db)
	if err != nil {
		return nil, err
	}
	findings = append(findings, roles...)

	sort.Strings(findings)
	return findings, nil
}

// checkPolicies judges only the policies that apply to *everyone*
// (polroles = '{0}'). A policy scoped to a named role is an exception by
// construction and is counted by checkExceptionInventories instead -- D3's
// membership_reader is deliberately `USING (true)` and would fail every test
// below if it were weighed as a general policy.
func checkPolicies(db *sql.DB, table string) ([]string, error) {
	rows, err := db.Query(`
		SELECT p.polname, p.polcmd,
		       pg_get_expr(p.polqual, p.polrelid),
		       coalesce(pg_get_expr(p.polwithcheck, p.polrelid), '')
		  FROM pg_policy p
		  JOIN pg_class c ON c.oid = p.polrelid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = 'public' AND c.relname = $1 AND p.polroles = '{0}'`, table)
	if err != nil {
		return nil, fmt.Errorf("reading policies on %s: %w", table, err)
	}
	defer rows.Close()

	want := fmt.Sprintf("(%s = app_current_org())", tenantColumnFor(table))
	var (
		findings    []string
		any         bool
		coversWrite bool
	)
	for rows.Next() {
		var name, cmd, using, withCheck string
		if err := rows.Scan(&name, &cmd, &using, &withCheck); err != nil {
			return nil, err
		}
		any = true

		if using != want {
			findings = append(findings, fmt.Sprintf(
				"%s: policy %q restricts rows with %q, want %q -- a policy that does not "+
					"name this table's tenant key isolates nothing",
				table, name, using, want))
			continue
		}

		// Postgres derives WITH CHECK from USING on a FOR ALL policy when it
		// is absent, so an empty one is equal rather than missing. Where it is
		// present it must match, or writes and reads disagree about who the
		// tenant is.
		if withCheck != "" && withCheck != using {
			findings = append(findings, fmt.Sprintf(
				"%s: policy %q checks writes with %q but reads with %q",
				table, name, withCheck, using))
		}

		// '*' is FOR ALL; 'a' insert, 'w' update. A table whose only policy is
		// FOR SELECT ('r') is readable and silently unwritable.
		if cmd == "*" || cmd == "a" || cmd == "w" {
			coversWrite = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if !any {
		findings = append(findings, fmt.Sprintf(
			"%s has no policy applying to all roles", table))
		return findings, nil
	}
	if !coversWrite {
		findings = append(findings, fmt.Sprintf(
			"%s has no policy covering writes, so inserts and updates are silently rejected", table))
	}
	return findings, nil
}

// checkExceptionInventories asserts the two deliberate exceptions to "row-level
// security decides what is visible", by count.
//
// These are not allowlist-driven on purpose. An allowlist is the right shape
// for a list that grows; the correct length of both of these is one, so a
// second of either is a security decision that must fail the build rather than
// be waved through by adding a line to a file.
func checkExceptionInventories(db *sql.DB) ([]string, error) {
	var findings []string

	// Exactly one role-scoped policy: membership_reader on memberships (D3).
	rows, err := db.Query(`
		SELECT c.relname, p.polname,
		       (SELECT string_agg(r.rolname, ',') FROM pg_roles r WHERE r.oid = ANY (p.polroles))
		  FROM pg_policy p
		  JOIN pg_class c ON c.oid = p.polrelid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = 'public' AND p.polroles <> '{0}'
		 ORDER BY c.relname, p.polname`)
	if err != nil {
		return nil, fmt.Errorf("reading role-scoped policies: %w", err)
	}
	defer rows.Close()

	var scoped []string
	for rows.Next() {
		var table, policy string
		var roles sql.NullString
		if err := rows.Scan(&table, &policy, &roles); err != nil {
			return nil, err
		}
		scoped = append(scoped, fmt.Sprintf("%s.%s (to %s)", table, policy, roles.String))
		// A role-scoped policy granted to the application role is not an
		// exception at all -- it is a general policy wearing a disguise.
		if strings.Contains(roles.String, "vekst_app") {
			findings = append(findings, fmt.Sprintf(
				"policy %s on %s is scoped to vekst_app, which is every request: "+
					"a role-scoped exception must not name the application role", policy, table))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(scoped) != 1 || !strings.HasPrefix(scoped[0], "memberships.membership_reader ") {
		findings = append(findings, fmt.Sprintf(
			"expected exactly one role-scoped policy (memberships.membership_reader, design D3), found %d: %v",
			len(scoped), scoped))
	}

	// Exactly one SECURITY DEFINER function vekst_app may execute:
	// orgs_for_user. A definer function runs as its owner, so a second one is
	// a second way to reach data the policy would otherwise have filtered.
	defRows, err := db.Query(`
		SELECT p.proname
		  FROM pg_proc p
		  JOIN pg_namespace n ON n.oid = p.pronamespace
		 WHERE n.nspname = 'public'
		   AND p.prosecdef
		   AND has_function_privilege('vekst_app', p.oid, 'EXECUTE')
		 ORDER BY p.proname`)
	if err != nil {
		return nil, fmt.Errorf("reading SECURITY DEFINER functions: %w", err)
	}
	defer defRows.Close()

	var definers []string
	for defRows.Next() {
		var name string
		if err := defRows.Scan(&name); err != nil {
			return nil, err
		}
		definers = append(definers, name)
	}
	if err := defRows.Err(); err != nil {
		return nil, err
	}
	if len(definers) != 1 || definers[0] != "orgs_for_user" {
		findings = append(findings, fmt.Sprintf(
			"expected exactly one SECURITY DEFINER function executable by vekst_app "+
				"(orgs_for_user, design D3), found %d: %v", len(definers), definers))
	}

	return findings, nil
}

// checkRoleAttributes is what keeps D0 true over time. Either attribute on
// either authenticating role bypasses row-level security unconditionally,
// FORCE included, which makes every assertion above decorative.
func checkRoleAttributes(db *sql.DB) ([]string, error) {
	var findings []string
	for _, role := range []string{"vekst_app", "vekst_migrator"} {
		var super, bypass bool
		err := db.QueryRow(
			`SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = $1`, role).Scan(&super, &bypass)
		if err != nil {
			return nil, fmt.Errorf("reading role %s: %w", role, err)
		}
		if super {
			findings = append(findings, role+" is a superuser, which bypasses row-level security unconditionally")
		}
		if bypass {
			findings = append(findings, role+" has BYPASSRLS")
		}
	}
	return findings, nil
}

// allowlistPath is the checked-in exemption list, relative to this package.
func allowlistPath() string {
	return filepath.Join("..", "..", "..", "deploy", "db", "rls-exempt-tables.txt")
}
