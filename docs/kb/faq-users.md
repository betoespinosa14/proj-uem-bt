---
title: FAQ: Users
section: FAQ
order: 100
summary: Adding people, roles, passwords, resetting two-step sign-in, removing users and handing over the owner role.
reviewed: 2026-11-20
---

### Who can manage users?

Anyone with `users.write`: owners and admins. Only owners can create, edit, delete or reset another **owner**, or give someone the owner role. See [Roles and permissions](roles.md).

### How do I invite someone?

There are no email invitations (BEACON doesn't send email). Instead:

1. **Users & roles → Add user**.
2. Enter their email, name and role, and a **temporary password** (at least 10 characters).
3. Give them the password in a secure way.
4. Ask them to turn on two-step sign-in under their name → **Account security**.

### Which role should I give?

| Give | To |
|---|---|
| Helpdesk | Support staff who run actions and scripts on devices |
| Auditor | People who only need to look, plus read the audit log |
| Admin | IT staff who manage rules, tasks, enrollment, users and integrations |
| Owner | Only the people ultimately responsible; at least two is wise |

Remember that helpdesk can run any ad-hoc script as root/SYSTEM on any device.

### Can people change their own password?

No, there's no "change my password" on the Account page. An owner or admin sets a new one: **Users & roles → edit (pencil) → New password**. Owners and admins can also change their own this way.

### Someone forgot their password. What do I do?

Set a new temporary password for them as above. There's no "forgot password" link. Changing a password signs them out everywhere and makes remembered browsers ask for a two-step code again.

### Someone lost their phone and recovery codes. How do I get them back in?

On **Users & roles**, click the shield icon (**Reset two-step sign-in**) on their row. This turns two-step sign-in off and signs them out everywhere. Their password alone then works until they turn it on again. Only owners can reset an owner. You can't reset your own here. If no owner or admin can sign in, see [Locked out](locked-out.md).

### How do I see who has two-step sign-in on?

The **2-step** badge next to the role on Users & roles.

### What does "Last sign-in" show?

When the user last completed a sign-in, or "Never".

### What happens when I change someone's role?

It takes effect at once: they're signed out everywhere and get the new permissions when they sign in again. The audit log records the old and new role.

### How do I remove someone?

The bin icon on their row. They're signed out immediately and can't sign in again. Their past actions stay in the audit log under their email. You can't remove yourself.

### How do I hand over the owner role?

There's no single "transfer" button. Do it in two steps:

1. As an owner, edit the new person and set their role to **Owner**.
2. Sign in as the **new** owner and change the old owner's role (for example to Admin), or remove them. The old owner can also lower their own role, but nobody can delete their own account.

An organization must always keep at least one owner, so the last owner can't be demoted.

**To fill in:** who the current owners are.

### Can one person be in two organizations?

No. Emails are unique on the whole server.

### Do users need anything installed?

No, just a web browser and, for two-step sign-in, an authenticator app (Google Authenticator, Microsoft Authenticator or similar).

### Can I force everyone to use two-step sign-in?

Not yet. It's on the roadmap. For now, check the badges and ask people to turn it on. See [Known security gaps](known-gaps.md).

### Is there single sign-on (Microsoft, Google)?

No. Sign-in is email and password only. SSO is on the roadmap.
