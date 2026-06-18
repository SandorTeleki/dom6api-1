CREATE TABLE IF NOT EXISTS items (
	id         INT  NOT NULL PRIMARY KEY,
	name       TEXT NOT NULL COLLATE NOCASE,
	type       TEXT NOT NULL CHECK(type IN ('1-h wpn','2-h wpn','shield','helm','crown','armor','boots','misc','barding','missile')),
	constlevel INT  NOT NULL,
	mainlevel  INT  NOT NULL,
	mpath      TEXT NOT NULL CHECK(LENGTH(mpath) <= 6),
	gemcost    TEXT NOT NULL CHECK(LENGTH(gemcost) <= 6)
);

CREATE TABLE IF NOT EXISTS mercs (
    id           INT  NOT NULL PRIMARY KEY,
    name         TEXT NOT NULL COLLATE NOCASE,
    bossname     TEXT NOT NULL COLLATE NOCASE,
    com INT  NOT NULL,
    unit      INT  NOT NULL,
    nrunits      INT  NOT NULL
);


CREATE TABLE IF NOT EXISTS sites (
	id      INT  NOT NULL PRIMARY KEY,
	name    TEXT NOT NULL COLLATE NOCASE,
	path    TEXT NOT NULL COLLATE NOCASE CHECK(path IN ('Fire','Air','Water','Earth','Astral','Death','Nature','Glamour','Blood','Holy')),
	level   INT  NOT NULL,
	rarity  TEXT NOT NULL COLLATE NOCASE CHECK(rarity IN ('Common','Uncommon','Rare','Never random','Throne lvl1','Throne lvl2','Throne lvl3'))
);


CREATE TABLE IF NOT EXISTS units (
	id    INT  NOT NULL PRIMARY KEY,
	name  TEXT NOT NULL COLLATE NOCASE,
	hp    INT  NOT NULL,
	size  INT  NOT NULL CHECK(size >= 1 AND size <= 10),
    mountmnr INT,
	coridermnr  INT
);

CREATE TABLE IF NOT EXISTS spells (
	id            INT  NOT NULL PRIMARY KEY,
	name          TEXT NOT NULL COLLATE NOCASE,
	gemcost       TEXT NOT NULL CHECK(LENGTH(gemcost) <= 6),
	mpath         TEXT NOT NULL CHECK(LENGTH(mpath) <= 6),
	type          TEXT NOT NULL CHECK(type IN ('Combat','Ritual')),
	school        TEXT NOT NULL CHECK(school IN ('Conjuration','Alteration','Evocation','Construction','Enchantment','Thaumaturgy','Blood','Divine')),
	researchlevel TEXT  NOT NULL
);

CREATE TABLE IF NOT EXISTS events (
    id INT NOT NULL PRIMARY KEY,
    name TEXT
);
