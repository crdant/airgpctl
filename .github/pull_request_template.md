<!--
PULL REQUEST TEMPLATE

This template follows a conversational, practitioner-focused style that combines technical
precision with approachable explanations. Write as an experienced developer sharing knowledge
with colleagues—direct and no-nonsense, but not dry or overly formal.

TITLE REQUIREMENTS (ALL must pass):
- Start with verb in third present tense (e.g., "Adds", "Fixes", "Improves")
- ≤ 40 characters total
- Present tense only (no "Added", "Will add", etc.)
- No first person ("I", "We", "My")
- Focus on INTENT (the business outcome or capability), not implementation details
- Good: "Tracks CMX credit purchases as deals" (what it enables)
- Bad: "Adds webhook handler and event listener" (how it's built)

BODY REQUIREMENTS:
- Use exact markdown headers below (case sensitive, correct dash count)
- TL;DR: 1-2 sentences maximum, start with DIFFERENT verb than title. State
  the intent (what problem gets solved or what becomes possible), not the
  mechanism.
- Details: Explain WHY the change was needed, your approach, and its impact.
  Implementation details are in the diff.
- No forbidden phrases: "this PR", "this change", "this commit", "this update"
- Present tense throughout
- Technical precision: include exact versions, commands, file paths in
  `backticks`

BEFORE SUBMITTING:
- [ ] Title ≤ 40 characters with verb in third person present tense (quick
  test: ends in "s")
- [ ] TL;DR starts with different verb than title, same tense
- [ ] No banned phrases in body
- [ ] Headers formatted exactly as shown below
- [ ] Tests pass
- [ ] Linting passes
-->

<!-- Replace this comment with your title following the requirements above -->

TL;DR
-----
<!-- Single sentence explaining the intent using a DIFFERENT verb than your title.
     State what problem gets solved or what becomes possible — not how it's built.
     Good: "Gives the revenue team automatic CRM visibility into credit purchases."
     Bad: "Implements a webhook handler that creates deal records via the REST API."
-->

Details
--------
<!-- Explain WHY this change was needed and its impact. Tell the story behind the change.
      What problem does it solve? What improvement does it enable?

      Focus on your INTENT, IMPACT, and APPROACH and not the implementation
      details. The PR text is supplemental to the diff, not a replacement for
      it.

      Include details that illuminate the purpose:
      - Link to relevant documentation or issues
      - Mention any trade-offs or limitations honestly


      If there are novel, clever, or tricky ideas in the code, mention those
      fragments in the PR text. You may quote snippets of the code to explain
      these areas. You may use small code blocks for these types of details
      ONLY.

      Version numbers, commands, identifies, and file paths should be in
      `backticks`.

      Use active voice and third person present tense throughout. Try to avoid
      bullet points and lists. This is a narrative to assist the reviewer and is
      the start of a conversation with then.

      SUBJECT GUIDANCE: Start sentences with either (1) an implied subject
      (the PR itself): "Creates a bucket with versioning disabled..." or
      (2) a concrete subject (actual infrastructure): "The bucket disables
      versioning since..." Avoid vague stand-ins like "the solution",
      "the change", "this PR"—these are just dressed-up ways of referring
      to the work itself and violate the "no forbidden phrases" rule.
 -->

<!-- Optional sections you can add if relevant:

## Testing
<!-- Describe how the change was tested, any new test cases added -->

## Breaking Changes
<!-- List any breaking changes and migration steps if applicable -->

## Dependencies
<!-- Note any new dependencies or version updates -->

-->
