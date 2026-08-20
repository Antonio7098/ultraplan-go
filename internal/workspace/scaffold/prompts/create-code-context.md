# Create Sprint Code Context

Explore the resolved implementation repository in depth, guided by the validated sprint requirements. Produce one concise, durable Markdown reference pack identifying the source locations that will help later agents reason about and plan the sprint.

Inspect enough implementation, boundary, configuration, error-handling, and test code to understand the current behavior before selecting references. Record precise repository-relative paths and line ranges and explain why each reference matters. Do not copy source code into the artifact; UltraPlan resolves the references and injects their current contents into downstream prompts.

Return only the complete `code-context.md` Markdown document. Do not make source or Git changes, write any other artifact, propose a final design, or produce an implementation plan.
