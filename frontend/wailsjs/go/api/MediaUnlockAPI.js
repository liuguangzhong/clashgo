'use strict';
import { Call as $Call } from "@wailsio/runtime";

export const CheckMediaUnlock = () => $Call.ByName("api.MediaUnlockAPI.CheckMediaUnlock");
