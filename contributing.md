<!-- omit in toc -->
# Contributing

First off, thanks for taking the time to contribute!

All types of contributions are encouraged and valued. See the [Table of Contents](#table-of-contents) for different ways to help and details about how this project handles them. Please read the relevant section before contributing. It makes it easier for the maintainer and smoother for everyone involved.

> And if you like the project, but just don't have time to contribute, that's fine. There are other easy ways to support the project and show your appreciation, which we would also be very happy about:
> - Star the project
> - Tweet/skeet/post about it
> - Refer this project in your project's readme
> - Mention the project to your friends/colleagues

<!-- omit in toc -->
## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Contributor License Agreement](#contributor-license-agreement)
- [I Have a Question](#i-have-a-question)
- [I Want To Contribute](#i-want-to-contribute)
	- [Reporting Bugs](#reporting-bugs)
	- [Suggesting Enhancements](#suggesting-enhancements)


## Code of Conduct

This project and everyone participating in it is governed by the
[nano-git-db Code of Conduct](https://github.com/jim-collier/nano-git-db/blob/main/code_of_conduct.md).
By participating, you are expected to uphold this code. Please report unacceptable behavior to <nano-git-db@ubx9.com>.


## Contributor License Agreement

nano-git-db is open source under the AGPLv3. A separately licensed edition is also offered on top of the same core. To keep that possible, every contribution must be covered by a signed [Contributor License Agreement](CLA.md). It lets the maintainer license your contribution under both the AGPL and other terms. You keep all rights to your own work. You only grant the maintainer the licenses described in the CLA.

Signing is automated. When you open your first pull request, a bot posts the CLA and asks you to sign with a one-line comment. Your signature is recorded once and applies to all future contributions. A pull request cannot be merged until its author has signed.


## I Have a Question

Before you ask a question, it is best to search for existing [Issues](https://github.com/jim-collier/nano-git-db/issues) that might help you. In case you have found a suitable issue and still need clarification, you can write your question in this issue. It is also advisable to search the internet for answers first.

If you then still feel the need to ask a question and need clarification, we recommend the following:

- Open an [Issue](https://github.com/jim-collier/nano-git-db/issues/new).
- Provide as much context as you can about what you're running into.
- Provide project and platform versions, depending on what seems relevant.

I'll then take care of the issue as soon as I can.

<!--
You might want to create a separate issue tag for questions and include it in this description. People should then tag their issues accordingly.

Depending on how large the project is, you may want to outsource the questioning, e.g. to Stack Overflow or Gitter. You may add additional contact and information possibilities:
- IRC
- Slack
- Gitter
- Stack Overflow tag
- Blog
- FAQ
- Roadmap
- E-Mail List
- Forum
-->

## I Want To Contribute

> ### Legal Notice <!-- omit in toc -->
> When contributing to this project, you must agree that you have authored 100% of the content, that you have the necessary rights to the content and that the content you contribute may be provided under the project license.

### Reporting Bugs

<!-- omit in toc -->
#### Before Submitting a Bug Report

A good bug report shouldn't leave others needing to chase you up for more information. Therefore, we ask you to investigate carefully, collect information and describe the issue in detail in your report. Please complete the following steps in advance to help us fix any potential bug as fast as possible.

- Make sure that you are using the latest version.
- Determine if your bug is really a bug and not an error on your side e.g. using incompatible environment components/versions (Make sure that you have read the [documentation](https://github.com/jim-collier/nano-git-db/blob/main/README.md). If you are looking for support, you might want to check [this section](#i-have-a-question)).
- To see if other users have experienced (and potentially already solved) the same issue you are having, check if there is not already a bug report existing for your bug or error in the [bug tracker](https://github.com/jim-collier/nano-git-db/issues?q=label%3Abug).
- Also make sure to search the internet (including Stack Overflow) to see if users outside of the GitHub community have discussed the issue.
- Collect information about the bug:
  - OS, Platform and Version (Windows, Linux, macOS, x86, ARM)
  - Environment versions
  - Possibly your input and the output
  - Can you reliably reproduce the issue? And can you also reproduce it with older versions?
  - Steps to cleanly reproduce from scratch.

<!-- omit in toc -->
#### How Do I Submit a Good Bug Report?

> You must never report security related issues, vulnerabilities or bugs including sensitive information to the issue tracker, or elsewhere in public. Instead sensitive bugs must be sent by email to <nano-git-db@ubx9.com>.
<!-- You may add a PGP key to allow the messages to be sent encrypted as well. -->

We use GitHub issues to track bugs and errors. If you run into an issue with the project:

- Open an [Issue](https://github.com/jim-collier/nano-git-db/issues/new). (Since we can't be sure at this point whether it is a bug or not, we ask you not to talk about a bug yet and not to label the issue.)
- Explain the behavior you would expect and the actual behavior.
- Please provide as much context as possible and describe the *reproduction steps* that someone else can follow to recreate the issue on their own. This usually includes your code. For good bug reports you should isolate the problem and create a reduced test case.
- Provide the information you collected in the previous section.

Once it's filed:

- The issue gets labeled accordingly.
- I'll try to reproduce it from your steps. If there are none, or no obvious way to reproduce it, I'll ask for steps and mark the issue `needs-repro`. Bugs tagged `needs-repro` are not addressed until they're reproduced.
- Once reproduced, it's marked `needs-fix` (plus other tags such as `critical` as warranted) and queued to be fixed.

<!-- You might want to create an issue template for bugs and errors that can be used as a guide and that defines the structure of the information to be included. If you do so, reference it here in the description. -->


### Suggesting Enhancements

This section guides you through submitting an enhancement suggestion for nano-git-db, **including completely new features and minor improvements to existing functionality**. Following these guidelines will help maintainers and the community to understand your suggestion and find related suggestions.

<!-- omit in toc -->
#### Before Submitting an Enhancement

- Make sure that you are using the latest version.
- Read the [documentation](https://github.com/jim-collier/nano-git-db/blob/main/README.md) carefully and find out if the functionality is already covered, maybe by an individual configuration.
- Perform a [search](https://github.com/jim-collier/nano-git-db/issues) to see if the enhancement has already been suggested. If it has, add a comment to the existing issue instead of opening a new one.
- Find out whether your idea fits with the scope and aims of the project. It's up to you to make a strong case to convince the project's developers of the merits of this feature. Keep in mind that we want features that will be useful to the majority of our users and not just a small subset. If you're just targeting a minority of users, consider writing an add-on/plugin library.

<!-- omit in toc -->
#### How Do I Submit a Good Enhancement Suggestion?

Enhancement suggestions are tracked as [GitHub issues](https://github.com/jim-collier/nano-git-db/issues).

- Use a **clear and descriptive title** for the issue to identify the suggestion.
- Provide a **step-by-step description of the suggested enhancement** in as many details as possible.
- **Describe the current behavior** and **explain which behavior you expected to see instead** and why. At this point you can also tell which alternatives do not work for you.
- **Explain why this enhancement would be useful** to most nano-git-db users. You may also want to point out the other projects that solved it better and which could serve as inspiration.

<!-- You might want to create an issue template for enhancement suggestions that can be used as a guide and that defines the structure of the information to be included. If you do so, reference it here in the description. -->

<!-- TODO
### Your First Code Contribution
include Setup of env, IDE and typical getting started instructions?

-->

<!-- TODO
### Improving The Documentation
Updating, improving and correcting the documentation

-->

<!-- TODO
## Styleguides
### Commit Messages

-->

<!-- TODO
## Join The Project Team
-->

<!-- omit in toc -->
## Attribution
This guide is based on the [contributing.md generator](https://contributing.md/generator)!
