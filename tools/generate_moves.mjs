import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// import official pokemon showdown moves.ts
const movesTsPath = process.argv[2] || process.env.SHOWDOWN_MOVES_TS || path.resolve(__dirname, '../../../pokemon-showdown/data/moves.ts');
const outputPath = process.argv[3] || path.resolve(__dirname, '../pkg/showdown/battle/data/moves.json');

const { Moves } = await import(movesTsPath);

const hazardConditions = new Set(['stealthrock', 'spikes', 'toxicspikes', 'stickyweb', 'gmaxsteelsurge']);
const screenConditions = new Set(['reflect', 'lightscreen', 'auroraveil', 'tailwind', 'safeguard', 'mist']);
const hazardClearingMoves = new Set(['rapidspin', 'mortalspin', 'defog', 'tidyup', 'courtchange']);

const healingMoves = new Set([
  'recover', 'softboiled', 'slackoff', 'roost', 'milkdrink',
  'synthesis', 'morningsun', 'moonlight', 'healorder', 'shoreup',
  'wish', 'rest', 'strengthsap', 'lifedew', 'junglehealing', 'purify'
]);

const statusAilmentMoves = {
  thunderwave: 'par',
  glare: 'par',
  stunspore: 'par',
  nuzzle: 'par',
  willowisp: 'brn',
  toxic: 'tox',
  poisonpowder: 'psn',
  poisonsting: 'psn',
  spore: 'slp',
  sleeppowder: 'slp',
  hypnosis: 'slp',
  sing: 'slp',
  grasswhistle: 'slp',
  darkvoid: 'slp',
  lovelykiss: 'slp',
  yawn: 'slp',
};

const enriched = {};

for (const [key, move] of Object.entries(Moves)) {
  const id = key.toLowerCase().replace(/[^a-z0-9]/g, '');
  if (!id) continue;

  const category = move.category || 'Physical';
  const name = move.name || key;
  const type = (move.type || 'Normal').toLowerCase();
  const basePower = typeof move.basePower === 'number' ? move.basePower : 0;
  const accuracy = typeof move.accuracy === 'number' ? move.accuracy : 0;
  const priority = typeof move.priority === 'number' ? move.priority : 0;

  // drain
  let drain = undefined;
  if (Array.isArray(move.drain) && move.drain.length === 2) {
    drain = [move.drain[0], move.drain[1]];
  }

  // recoil
  let recoil = undefined;
  if (Array.isArray(move.recoil) && move.recoil.length === 2) {
    recoil = [move.recoil[0], move.recoil[1]];
  }

  // sideCondition
  let sideCondition = move.sideCondition ? String(move.sideCondition).toLowerCase().replace(/[^a-z0-9]/g, '') : undefined;

  // status
  let status = move.status ? String(move.status).toLowerCase() : undefined;
  if (!status && statusAilmentMoves[id]) {
    status = statusAilmentMoves[id];
  }

  // volatileStatus
  const volatileStatus = move.volatileStatus ? String(move.volatileStatus).toLowerCase() : undefined;

  // selfBoosts
  let selfBoosts = undefined;
  if (move.self && move.self.boosts) {
    selfBoosts = { ...move.self.boosts };
  } else if (move.boosts && (move.target === 'self' || move.target === 'adjacentAllyOrSelf' || category === 'Status')) {
    selfBoosts = { ...move.boosts };
  }

  // target boosts
  let boosts = undefined;
  if (move.boosts && move.target !== 'self' && move.target !== 'adjacentAllyOrSelf' && category !== 'Status') {
    boosts = { ...move.boosts };
  }

  // secondary
  let secondary = undefined;
  const sec = move.secondary || (Array.isArray(move.secondaries) && move.secondaries[0] ? move.secondaries[0] : null);
  if (sec) {
    const chance = typeof sec.chance === 'number' ? sec.chance : 100;
    const secStatus = sec.status ? String(sec.status).toLowerCase() : undefined;
    const secBoosts = sec.boosts ? { ...sec.boosts } : undefined;
    const secSelfBoosts = sec.self && sec.self.boosts ? { ...sec.self.boosts } : undefined;

    if (secStatus || secBoosts || secSelfBoosts) {
      secondary = {
        chance,
        ...(secStatus ? { status: secStatus } : {}),
        ...(secBoosts ? { boosts: secBoosts } : {}),
        ...(secSelfBoosts ? { selfBoosts: secSelfBoosts } : {}),
      };
    }

    // if secondary is 100% chance, also integrate into main boosts / selfBoosts for deterministic minimax
    if (chance === 100) {
      if (secSelfBoosts) {
        selfBoosts = { ...(selfBoosts || {}), ...secSelfBoosts };
      }
      if (secBoosts) {
        boosts = { ...(boosts || {}), ...secBoosts };
      }
    }
  }

  // special case for tidyup, bellydrum, etc
  if (id === 'tidyup') {
    selfBoosts = { ...(selfBoosts || {}), atk: 1, spe: 1 };
  }
  if (id === 'bellydrum') {
    selfBoosts = { atk: 6 };
  }
  if (id === 'mortalspin') {
    secondary = { chance: 100, status: 'psn' };
  }
  if (id === 'ceaselessedge') {
    sideCondition = 'spikes';
  }
  if (id === 'stoneaxe') {
    sideCondition = 'stealthrock';
  }

  const isHealing = !!(move.flags && move.flags.heal) || healingMoves.has(id) || !!drain;
  const isHazard = (sideCondition && hazardConditions.has(sideCondition)) || false;

  let isSetup = false;
  if (selfBoosts) {
    for (const v of Object.values(selfBoosts)) {
      if (typeof v === 'number' && v > 0) {
        isSetup = true;
        break;
      }
    }
  }

  const isStatus = category === 'Status' && (!!status || id in statusAilmentMoves);
  const clearsHazards = hazardClearingMoves.has(id);
  const forceSwitch = !!move.forceSwitch;

  enriched[id] = {
    id,
    name,
    type,
    category,
    basePower,
    accuracy,
    priority,
    isHealing,
    isHazard,
    isSetup,
    isStatus,
    ...(selfBoosts && Object.keys(selfBoosts).length > 0 ? { selfBoosts } : {}),
    ...(boosts && Object.keys(boosts).length > 0 ? { boosts } : {}),
    ...(sideCondition ? { sideCondition } : {}),
    ...(drain ? { drain } : {}),
    ...(recoil ? { recoil } : {}),
    ...(status ? { status } : {}),
    ...(volatileStatus ? { volatileStatus } : {}),
    ...(forceSwitch ? { forceSwitch } : {}),
    ...(secondary ? { secondary } : {}),
    ...(clearsHazards ? { clearsHazards: true } : {}),
  };
}

fs.writeFileSync(outputPath, JSON.stringify(enriched, null, 2));
console.log(`Successfully generated ${Object.keys(enriched).length} enriched moves to ${outputPath}`);
