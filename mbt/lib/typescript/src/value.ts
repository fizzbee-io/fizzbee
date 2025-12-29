/**
 * Utilities for converting between TypeScript values and protobuf Value messages.
 */

import * as pb from '../proto-gen/mbt_plugin_pb';
import { Ignored } from './sentinels';

/**
 * Converts a protobuf Value to a native TypeScript value.
 */
export function fromProtoValue(protoValue: pb.Value | undefined): any {
  if (!protoValue) {
    return null;
  }

  const kindCase = protoValue.getKindCase();

  switch (kindCase) {
    case pb.Value.KindCase.STR_VALUE:
      return protoValue.getStrValue();

    case pb.Value.KindCase.INT_VALUE:
      return protoValue.getIntValue();

    case pb.Value.KindCase.BOOL_VALUE:
      return protoValue.getBoolValue();

    case pb.Value.KindCase.MAP_VALUE: {
      const mapValue = protoValue.getMapValue();
      if (!mapValue) return {};

      const result: Record<any, any> = {};
      const entries = mapValue.getEntriesList();
      for (const entry of entries) {
        const key = fromProtoValue(entry.getKey());
        const value = fromProtoValue(entry.getValue());
        result[key] = value;
      }
      return result;
    }

    case pb.Value.KindCase.LIST_VALUE: {
      const listValue = protoValue.getListValue();
      if (!listValue) return [];

      const items = listValue.getItemsList();
      return items.map(item => fromProtoValue(item));
    }

    default:
      return null;
  }
}

/**
 * Converts a native TypeScript value to a protobuf Value.
 */
export function toProtoValue(value: any): pb.Value {
  const result = new pb.Value();

  // Check for Ignored instances FIRST
  if (value instanceof Ignored) {
    result.setSentinelValue(pb.SentinelType.SENTINEL_IGNORE);
    return result;
  }

  // Check for sentinel symbols (before null/undefined check)
  if (typeof value === 'symbol') {
    const sentinelType = symbolToSentinelType(value);
    if (sentinelType !== null) {
      result.setSentinelValue(sentinelType);
      return result;
    }
  }

  if (value === null || value === undefined) {
    return result;
  }

  const valueType = typeof value;

  if (valueType === 'string') {
    result.setStrValue(value);
    return result;
  }

  if (valueType === 'number') {
    result.setIntValue(Math.floor(value));
    return result;
  }

  if (valueType === 'boolean') {
    result.setBoolValue(value);
    return result;
  }

  if (Array.isArray(value)) {
    const listValue = new pb.ListValue();
    const items = value.map(item => toProtoValue(item));
    listValue.setItemsList(items);
    result.setListValue(listValue);
    return result;
  }

  // Handle Map instances (can have symbol keys like IGNORE)
  if (value instanceof Map) {
    const mapValue = new pb.MapValue();
    const entries = Array.from(value.entries())
      .map(([k, v]) => {
        const entry = new pb.MapEntry();
        entry.setKey(toProtoValue(k));
        entry.setValue(toProtoValue(v));
        return entry;
      })
      .sort((a, b) => {
        const aStr = JSON.stringify(a.toObject());
        const bStr = JSON.stringify(b.toObject());
        return aStr.localeCompare(bStr);
      });

    mapValue.setEntriesList(entries);
    result.setMapValue(mapValue);
    return result;
  }

  if (valueType === 'object') {
    const mapValue = new pb.MapValue();
    const entries = Object.entries(value)
      .map(([k, v]) => {
        const entry = new pb.MapEntry();
        entry.setKey(toProtoValue(k));
        entry.setValue(toProtoValue(v));
        return entry;
      })
      .sort((a, b) => {
        const aStr = JSON.stringify(a.toObject());
        const bStr = JSON.stringify(b.toObject());
        return aStr.localeCompare(bStr);
      });

    mapValue.setEntriesList(entries);
    result.setMapValue(mapValue);
    return result;
  }

  return result;
}

/**
 * Converts a protobuf Arg to a native Arg.
 */
export function fromProtoArg(protoArg: pb.Arg): { name: string; value: any } {
  return {
    name: protoArg.getName(),
    value: fromProtoValue(protoArg.getValue())
  };
}

/**
 * Converts an array of protobuf Args to native args.
 */
export function fromProtoArgs(protoArgs: pb.Arg[]): any[] {
  if (!protoArgs) {
    return [];
  }
  return protoArgs.map(arg => fromProtoArg(arg).value);
}

/**
 * Converts a TypeScript symbol to protobuf SentinelType enum value.
 * Returns null if the symbol is not a recognized sentinel.
 */
function symbolToSentinelType(sym: symbol): number | null {
  const key = Symbol.keyFor(sym);
  switch (key) {
    case 'fizzbee.mbt.IGNORE':
      return pb.SentinelType.SENTINEL_IGNORE;
    default:
      return null;
  }
}
