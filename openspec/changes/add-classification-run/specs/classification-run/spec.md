# classification-run Specification

## Purpose

This capability turns a persisted batch's transactions into classifications, or leaves
each one for the review queue, and records the outcome on a run row a screen can read.

## ADDED Requirements

### Requirement: A batch is classified once it is imported

The system SHALL enqueue exactly one classification run when an import batch reaches
`imported`, and SHALL refuse a second run for the same batch.

#### Scenario: A newly imported batch is classified

- **WHEN** a batch's status becomes `imported`
- **THEN** a classification run for that batch is created in `running`

#### Scenario: A second run for the same batch is refused

- **WHEN** a classification run already exists for a batch
- **THEN** creating another for the same batch is refused

### Requirement: Classification proceeds in atomic chunks

The system SHALL page through a batch's unclassified transactions in chunks of at most
5,000, and SHALL write a chunk's classifications in one transaction, together with the
run row's own counters.

#### Scenario: A chunk's classifications commit together

- **WHEN** the classifier returns proposals for a chunk
- **THEN** every classification from that chunk is stored together with the run's updated
  counts, or none of them are

#### Scenario: A failed chunk writes nothing

- **WHEN** writing a chunk's classifications fails partway through
- **THEN** no classification from that chunk is stored
- **AND** the transactions in that chunk remain unclassified for the next attempt

### Requirement: An unreachable or erroring classifier is retried, never guessed around

The system SHALL retry a chunk with backoff when the classifier is unreachable, and SHALL
fail the run rather than write partial results when the classifier returns an error.

#### Scenario: The classifier is temporarily unreachable

- **WHEN** the classifier does not respond
- **THEN** the run stays `running`
- **AND** the chunk is retried with backoff

#### Scenario: The classifier returns an error

- **WHEN** the classifier's response for a chunk is an error
- **THEN** the run is marked `failed` with a failure code
- **AND** no classification from that chunk is stored

### Requirement: An unknown category is a rejected response, never a guess

The system SHALL reject a classifier response naming a `category_id` this organisation
cannot resolve, in its entirety, rather than storing the proposals it can resolve.

#### Scenario: A response names an unknown category

- **WHEN** a chunk's response includes a proposal for a `category_id` that does not
  resolve
- **THEN** the whole chunk's response is rejected
- **AND** no classification from that chunk is stored

### Requirement: Below-threshold proposals are left for review, not treated as failures

The system SHALL leave a transaction with no live classification when its proposal's
confidence is below the organisation's threshold, and SHALL NOT treat this as a run
failure.

#### Scenario: A low-confidence proposal is not stored

- **WHEN** the classifier's proposal for a transaction is below threshold
- **THEN** that transaction has no live classification afterward
- **AND** the run counts it as needing review, not as failed

### Requirement: A run's outcome is visible on the batch

The system SHALL record `running`, `classified` or `failed` on the batch's classification
run, along with how many transactions were classified and how many were left for review.

#### Scenario: A run completes successfully

- **WHEN** every chunk of a batch classifies without error
- **THEN** the run is marked `classified`
- **AND** its classified and review counts reflect every transaction in the batch

### Requirement: Classification runs are isolated between organisations

The system SHALL apply row-level security to classification runs, with `FORCE ROW LEVEL
SECURITY`, and SHALL raise rather than return an empty result when no tenant context is
set.

#### Scenario: One organisation cannot read another's run

- **WHEN** a member of organisation A reads organisation B's classification run
- **THEN** the result is indistinguishable from the run not existing

#### Scenario: A read without tenant context raises

- **WHEN** a classification-run query runs outside a tenant transaction
- **THEN** the database raises `42704`
- **AND** it does not return zero rows
