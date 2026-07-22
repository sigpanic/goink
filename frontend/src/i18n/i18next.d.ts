// Type augmentation for i18next
// Provides autocompletion for literal key strings while still
// allowing dynamic t(variable) patterns used in data-driven components

import type zhCN from "./locales/zh-CN.json";

// Extract all leaf key paths from the translation resource
type LeafKeys<T, Prefix extends string = ""> = T extends string
  ? Prefix
  : T extends object
    ? {
        [K in keyof T & string]: LeafKeys<
          T[K],
          Prefix extends "" ? K : `${Prefix}.${K}`
        >;
      }[keyof T & string]
    : never;

type TypedKeys = LeafKeys<typeof zhCN>;

declare module "i18next" {
  interface CustomTypeOptions {
    defaultNS: "translation";
    resources: {
      translation: typeof zhCN;
    };
    keySeparator: ".";
  }

  // Override the TFunction type to accept both typed keys and plain strings
  // This preserves autocompletion for literal keys while allowing dynamic patterns
  interface TFunction {
    <Key extends string = string, TInterpolationMap extends object = object>(
      key: Key | Key[],
      options?: TOptionsBase & TInterpolationMap & $Dictionary,
    ): string;
    <Key extends string = string, TInterpolationMap extends object = object>(
      key: Key | Key[],
      defaultValue?: string,
      options?: TOptionsBase & TInterpolationMap & $Dictionary,
    ): string;
  }
}
