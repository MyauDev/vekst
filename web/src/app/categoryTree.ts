/**
 * The review picker's taxonomy, as a tree rather than a flat list of leaves.
 *
 * A flat list cannot tell five leaves called "Salary" apart: they differ only
 * by the department above them, and the department is exactly what the flat
 * list threw away. The tree is built from `Category.path`, which the backend
 * already sends for every leaf -- no second call, no ancestor rows on the wire.
 *
 * Names on the way down are translated from the ancestor's code, which is the
 * leaf's code cut to two characters per level (`00005_classification_taxonomy`
 * makes that the rule, and the seed depends on it). A leaf whose code does not
 * line up with its path -- an org-scoped addition this build never heard of --
 * keeps the wire's own names, the same fallback `categoryName` makes.
 *
 * Below the top level, a branch with a single child is folded into it: "FI
 * Expenses" holds nothing but "Salary", and a click that can only ever lead to
 * one place is a click nobody needs. The folded node keeps both names
 * ("Expenses › Salary") so nothing is hidden. The sections themselves are never
 * folded -- "Net sales" is the answer to "income or expense", and replacing it
 * with its only leaf would remove the one step that says which side this is.
 */
import type { Category } from "../data/review";
import type { Locale } from "../i18n";
import { branchName, categoryName } from "./categoryName";
import { lineName } from "./lineName";

export const SEPARATOR = " › ";

export interface TreeLeaf {
  kind: "leaf";
  key: string;
  code: string;
  /** The leaf's own name. */
  name: string;
  /** What its parent shows: `name`, or a folded chain ending in it. */
  label: string;
  /** Every ancestor's name, section first, nothing folded. */
  trail: readonly string[];
}

export interface TreeBranch {
  kind: "branch";
  key: string;
  label: string;
  children: TreeNode[];
}

export type TreeNode = TreeLeaf | TreeBranch;

/** Which way money moves through a category. */
export type Side = "income" | "expense" | "both";

/**
 * The seed's sections and the branches that split a mixed section, by code
 * prefix. The taxonomy itself carries no such flag -- a section's name says
 * it ("Net sales", "Operating expenses"), and so do the rules, which match
 * `direction` before they ever name one of these. Other income and expenses
 * (05) and the financial result (06) hold both sides, so they are decided one
 * level down; exchange differences go either way, and so does anything this
 * table has never heard of -- an org-scoped section is offered on both sides
 * rather than hidden from one.
 */
const SIDES: readonly (readonly [string, Side])[] = [
  ["01", "income"],
  ["0502", "income"],
  ["0602", "income"],
  ["02", "expense"],
  ["03", "expense"],
  ["04", "expense"],
  ["0501", "expense"],
  ["07", "expense"],
  ["08", "expense"],
];

export function sideOf(code: string): Side {
  return SIDES.find(([prefix]) => code.startsWith(prefix))?.[1] ?? "both";
}

function segmentsOf(path: string): string[] {
  return path
    .split(">")
    .map((s) => s.trim())
    .filter((s) => s !== "");
}

export function buildCategoryTree(categories: readonly Category[], locale: Locale): TreeBranch {
  const root: TreeBranch = { kind: "branch", key: "", label: "", children: [] };
  const branches = new Map<string, TreeBranch>();

  for (const c of categories) {
    const segments = segmentsOf(c.path);
    // The path names the leaf itself last. A path that does not -- or no path
    // at all -- still puts the leaf somewhere rather than nowhere.
    const ancestors = segments.length > 0 ? segments.slice(0, -1) : [];
    const aligned = c.code.length === 2 * segments.length;

    let parent = root;
    const trail: string[] = [];
    for (let i = 0; i < ancestors.length; i++) {
      const raw = ancestors[i]!;
      const code = aligned ? c.code.slice(0, 2 * (i + 1)) : undefined;
      const label = code === undefined ? raw : i === 0 ? lineName(code, raw, locale) : branchName(code, raw, locale);
      const key = `${parent.key}/${code ?? raw}`;
      let branch = branches.get(key);
      if (!branch) {
        branch = { kind: "branch", key, label, children: [] };
        branches.set(key, branch);
        parent.children.push(branch);
      }
      trail.push(label);
      parent = branch;
    }
    const name = categoryName(c.code, c.label, locale);
    parent.children.push({ kind: "leaf", key: `${parent.key}/${c.code}`, code: c.code, name, label: name, trail });
  }

  for (const section of root.children) {
    if (section.kind === "branch") fold(section);
  }
  return root;
}

/** Folds every single-child chain under `branch`, never `branch` itself. */
function fold(branch: TreeBranch): void {
  branch.children = branch.children.map((child) => {
    let node = child;
    while (node.kind === "branch" && node.children.length === 1) {
      const only = node.children[0]!;
      node = { ...only, label: node.label + SEPARATOR + only.label };
    }
    if (node.kind === "branch") fold(node);
    return node;
  });
}

export function leavesOf(node: TreeNode): TreeLeaf[] {
  return node.kind === "leaf" ? [node] : node.children.flatMap(leavesOf);
}

/** The branches `keys` names, root excluded, stopping at the first that no
 *  longer exists -- a taxonomy reloaded under a different locale or version
 *  keeps whatever prefix of the open path still means something. */
export function openPath(root: TreeBranch, keys: readonly string[]): TreeBranch[] {
  const out: TreeBranch[] = [];
  let cur = root;
  for (const key of keys) {
    const next = cur.children.find((c): c is TreeBranch => c.kind === "branch" && c.key === key);
    if (!next) break;
    out.push(next);
    cur = next;
  }
  return out;
}
