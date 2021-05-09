// SPDX-FileCopyrightText: 2021 Softbear, Inc.
// SPDX-License-Identifier: AGPL-3.0-or-later

import fs from 'fs';
import {mapRanges} from '../util/math.js';

const entityDatas = JSON.parse(fs.readFileSync('./entities-raw.json'));

for (const entityType of Object.keys(entityDatas)) {
	const entityData = entityDatas[entityType];

	if (entityData.range) {
		let maxRange = 1500;
		if (entityData.type === 'weapon') {
			switch (entityData.subtype) {
				case 'shell':
					maxRange = mapRanges(entityData.length, 0.2, 2, 250, 800, true);
					break;
				case 'rocket':
				case 'missile':
					maxRange = mapRanges(entityData.length, 1, 10, 400, maxRange, true);
					break;
			}
		}
		entityData.range = Math.min(entityData.range, maxRange);
	}

	if (entityData.type === 'weapon' && entityData.damage == undefined) {
		switch (entityData.subtype) {
			case 'torpedo':
				entityData.damage = mapRanges(entityData.length, 3, 7, 0.6, 1.1, true);
				/*
				if (Array.isArray(entityData.sensors) && entityData.sensors.length > 0) {
					entityData.damage -= 0.1;
				}
				*/
				break;
			case 'rocket':
			case 'missile':
				entityData.damage = mapRanges(entityData.length, 1, 6, 0.15, 0.9, true);
				break;
			case 'shell':
				entityData.damage =  mapRanges(entityData.length, 0.25, 2, 0.3, 0.8, true);
				break;
			case 'depthCharge':
				entityData.damage = 1;
				break;
		}
	}

	const armaments = entityData.armaments;
	entityData.armaments = [];

	const turrets = entityData.turrets;
	entityData.turrets = [];

	if (turrets) {
		for (let i = 0; i < turrets.length; i++) {
			const turret = turrets[i];
			turret.angle = (turret.angle || 0) * Math.PI / 180;
			const sym = turret.symmetrical;
			delete turret.symmetrical;
			entityData.turrets.push(turret);

			if (sym) {
				const copy = {...turret, angle: -turret.angle, positionSide: -turret.positionSide};
				entityData.turrets.push(copy);
			}
		}

		for (let i = 0; i < entityData.turrets.length; i++) {
			const turret = entityData.turrets[i];

			if (turret.type) {
				for (const armament of entityDatas[turret.type].armaments) {
					armaments.push({...armament, turret: i});
				}
			}
		}
	}

	if (armaments) {
		for (const armament of armaments) {
			armament.angle = armament.angle * Math.PI / 180;
			const sym = armament.symmetrical;
			delete armament.symmetrical;
			entityData.armaments.push(armament);
			if (sym) {
				const copy = {...armament, angle: -armament.angle, positionSide: -armament.positionSide};
				entityData.armaments.push(copy);
			}
		}
	}

	const sensors = entityData.sensors;
	entityData.sensors = [];

	if (sensors) {
		for (const sensor of sensors) {
			if (sensor.range) {
				sensor.range = Math.min(sensor.range, 2000);
			}
			entityData.sensors.push(sensor);
		}
	}
}

fs.writeFileSync('./entities.json', JSON.stringify(entityDatas));