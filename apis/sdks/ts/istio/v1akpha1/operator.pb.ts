/* eslint-disable */
// @ts-nocheck
/*
* This file is a generated Typescript file for GRPC Gateway, DO NOT MODIFY
*/

import * as GoogleProtobufStruct from "../../google/protobuf/struct.pb"

export enum InstallStatusStatus {
  NONE = "NONE",
  UPDATING = "UPDATING",
  RECONCILING = "RECONCILING",
  HEALTHY = "HEALTHY",
  ERROR = "ERROR",
  ACTION_REQUIRED = "ACTION_REQUIRED",
}

export type IstioOperatorSpec = {
  profile?: string
  installPackagePath?: string
  hub?: string
  tag?: GoogleProtobufStruct.Value
  resourceSuffix?: string
  namespace?: string
  revision?: string
  compatibilityVersion?: string
  meshConfig?: GoogleProtobufStruct.Struct
  components?: GoogleProtobufStruct.Struct
  values?: GoogleProtobufStruct.Struct
  unvalidatedValues?: GoogleProtobufStruct.Struct
}

export type InstallStatusVersionStatus = {
  version?: string
  status?: InstallStatusStatus
  error?: string
}

export type InstallStatus = {
  status?: InstallStatusStatus
  message?: string
  componentStatus?: {[key: string]: InstallStatusVersionStatus}
}