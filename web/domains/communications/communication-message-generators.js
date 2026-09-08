// Pure text-building functions for the League Communication screen (Phase
// 1). Each function takes already-fetched data (a week recap, a finances
// dues response, a player overview) and returns a plain-text message ready
// to copy/paste -- no fetching, no DOM, no business-rule computation of its
// own. The status/paid/approved/processed/closed *decisions* all come from
// the backend response fields; these functions only choose display wording
// for values the server already computed, the same division of
// responsibility the Weekly Summary and Financial screens already use for
// their badges.

function fmtHC(v) {
  const n = Number(v) || 0;
  return (n >= 0 ? '+' : '') + n;
}

function fmtMoney(v) {
  const n = Number(v) || 0;
  return '$' + n.toFixed(2);
}

// matchStatusLabel mirrors weekly-summary-page-component.js's #matchStatus
// badge mapping exactly (Unscored / Scored / Approved / Processed /
// Closed), duplicated here as plain text rather than imported, since that
// method is private to that component's class.
function matchStatusLabel(m) {
  if (m.week_closed) return 'Closed';
  if (m.processed_at) return 'Processed';
  if (m.approved_at) return 'Approved';
  if (m.has_result) return 'Scored';
  return 'Unscored';
}

function matchStatusReminder(m) {
  if (m.week_closed) return 'This week is officially closed -- no further changes expected.';
  if (m.processed_at) return 'Your match has been processed. Handicap changes, if any, will follow from this result.';
  if (m.approved_at) return 'Your match has been approved and is awaiting weekly processing.';
  if (m.has_result) return 'Scores are in and awaiting admin approval.';
  return 'Scores have not been entered yet. Please make sure your lineup is set and scores are submitted after play.';
}

// buildWeeklyTeamSummaryMessage generates a per-team update for one
// season/week, built from a GET .../weeks/{week}/recap response (already
// fetched for every team's matches, filtered here to the ones involving
// the selected team -- normally 0 or 1 in a round-robin schedule, but this
// handles more than one without assuming the shape).
export function buildWeeklyTeamSummaryMessage({ team, season, weekNumber, recap }) {
  const lines = [];
  lines.push(`Weekly Update - ${team.name}`);
  lines.push(`${season.name}, Week ${weekNumber}`);
  lines.push('');

  const matches = (recap.matches || []).filter(m =>
    m.home_team_id === team.id || m.away_team_id === team.id
  );

  if (matches.length === 0) {
    lines.push(`No match scheduled for ${team.name} in Week ${weekNumber}.`);
  } else {
    for (const m of matches) {
      const isHome = m.home_team_id === team.id;
      const opponent = isHome ? (m.away_team_name || '(unassigned)') : (m.home_team_name || '(unassigned)');
      lines.push(`${team.name} vs ${opponent} (${isHome ? 'Home' : 'Away'})`);
      lines.push(`Status: ${matchStatusLabel(m)}`);
      if (m.has_result) {
        const teamSets = isHome ? m.home_sets_won : m.away_sets_won;
        const oppSets = isHome ? m.away_sets_won : m.home_sets_won;
        lines.push(`Result: ${teamSets} sets - ${oppSets} sets`);
      }
      lines.push('');
      lines.push(matchStatusReminder(m));
      lines.push('');
    }
  }

  if (recap.missing_count > 0) {
    lines.push(`Note: ${recap.missing_count} other match${recap.missing_count !== 1 ? 'es' : ''} in the league still need${recap.missing_count === 1 ? 's' : ''} scores this week.`);
    lines.push('');
  }

  if (recap.next_week_number) {
    const nw = recap.next_week || {};
    lines.push(`Looking ahead: Week ${recap.next_week_number} has ${nw.match_count || 0} match${(nw.match_count || 0) !== 1 ? 'es' : ''} scheduled.`);
  }

  return lines.join('\n').trim() + '\n';
}

// buildTeamDuesReminderMessage generates a per-team dues status message
// from a GET .../finances/dues response, filtered to the selected team's
// players.
export function buildTeamDuesReminderMessage({ team, season, dues }) {
  const lines = [];
  lines.push(`Dues Reminder - ${team.name}`);
  lines.push(`${season.name}`);
  lines.push('');
  lines.push(`Dues amount: ${dues.dues_amount ? esc0(dues.dues_amount) : 'not set'}`);
  lines.push('');

  const players = (dues.players || [])
    .filter(p => p.team_id === team.id)
    .sort((a, b) => a.player_name.localeCompare(b.player_name));

  if (players.length === 0) {
    lines.push('No rostered players found for this team this season.');
    return lines.join('\n').trim() + '\n';
  }

  const unpaid = [];
  for (const p of players) {
    if (p.paid) {
      lines.push(`- ${p.player_name}: Paid (${fmtMoney(p.total_paid)} total)`);
    } else {
      lines.push(`- ${p.player_name}: UNPAID`);
      unpaid.push(p.player_name);
    }
  }
  lines.push('');

  if (unpaid.length === 0) {
    lines.push(`Everyone on ${team.name} is paid up. Thanks!`);
  } else {
    lines.push(`Please remind: ${unpaid.join(', ')}.`);
  }

  return lines.join('\n').trim() + '\n';
}

// esc0 is a tiny passthrough kept distinct from the DOM-escaping esc()
// used by page components -- this module only ever produces plain text
// (no HTML), so no escaping is actually needed, but the value still comes
// from a season_rules freeform string and is rendered as-is here.
function esc0(s) { return String(s ?? ''); }

// buildPlayerSummaryMessage generates one player's schedule/stats/dues
// summary from a GET /players/{id}/overview response (Player Overview's
// existing aggregate -- no new fields needed).
export function buildPlayerSummaryMessage({ overview }) {
  const p = overview.player;
  const lines = [];
  lines.push(`Player Summary - ${p.name}`);
  lines.push(`${overview.team ? overview.team.name : 'No team'}, ${overview.season.name}`);
  lines.push('');
  lines.push(`Current handicap: ${fmtHC(overview.handicap.current)}`);
  lines.push(`Record: ${overview.stats.sets_won}-${overview.stats.sets_lost} sets, ` +
    `${overview.stats.games_won}-${overview.stats.games_lost} games ` +
    `(Win % ${(overview.stats.win_pct * 100).toFixed(1)}%)`);
  lines.push('');

  lines.push('Schedule:');
  const schedule = overview.schedule || [];
  if (schedule.length === 0) {
    lines.push('No scheduled matches this season.');
  } else {
    for (const m of schedule) {
      lines.push(`Week ${m.week_number} - ${m.match_date || 'TBD'} vs ${m.opponent_team_name} ` +
        `(${m.home_or_away === 'home' ? 'Home' : 'Away'}) - ${m.completed ? 'Completed' : 'Pending'}`);
    }
  }
  lines.push('');

  const money = overview.money || {};
  if (!money.tracked) {
    lines.push(`Dues status: ${money.message || 'Dues are not tracked yet.'}`);
  } else if (money.paid) {
    const latest = money.payments && money.payments.length ? money.payments[0] : null;
    lines.push(`Dues status: Paid (${fmtMoney(money.total_paid)} total` +
      (latest ? `, last payment ${latest.paid_at}` : '') + ')');
  } else {
    lines.push('Dues status: Unpaid');
  }

  return lines.join('\n').trim() + '\n';
}
