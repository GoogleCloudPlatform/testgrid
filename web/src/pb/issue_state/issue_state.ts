// @generated by protobuf-ts 2.8.1
// @generated from protobuf file "pb/issue_state/issue_state.proto" (syntax proto3)
// tslint:disable
//
// Backing state for issues associated with a TestGrid test group.
//
import type { BinaryWriteOptions } from '@protobuf-ts/runtime';
import type { IBinaryWriter } from '@protobuf-ts/runtime';
import { WireType } from '@protobuf-ts/runtime';
import type { BinaryReadOptions } from '@protobuf-ts/runtime';
import type { IBinaryReader } from '@protobuf-ts/runtime';
import { UnknownFieldHandler } from '@protobuf-ts/runtime';
import type { PartialMessage } from '@protobuf-ts/runtime';
import { reflectionMergePartial } from '@protobuf-ts/runtime';
import { MESSAGE_TYPE } from '@protobuf-ts/runtime';
import { MessageType } from '@protobuf-ts/runtime';
/**
 * @generated from protobuf message TargetAndMethods
 */
export interface TargetAndMethods {
  /**
   * @generated from protobuf field: string target_name = 1;
   */
  targetName: string;
  /**
   * @generated from protobuf field: repeated string method_names = 2;
   */
  methodNames: string[];
}
/**
 * @generated from protobuf message IssueInfo
 */
export interface IssueInfo {
  /**
   * @generated from protobuf field: string issue_id = 1;
   */
  issueId: string;
  /**
   * @generated from protobuf field: string title = 2;
   */
  title: string; // Issue title or description.
  /**
   * @generated from protobuf field: bool is_autobug = 3;
   */
  isAutobug: boolean; // True if auto-created by TestGrid for a failing test.
  /**
   * @generated from protobuf field: bool is_flakiness_bug = 8;
   */
  isFlakinessBug: boolean; // True if auto-created by TestGrid for a flaky test.
  /**
   * @generated from protobuf field: double last_modified = 4;
   */
  lastModified: number; // In seconds since epoch.
  /**
   * @generated from protobuf field: repeated string row_ids = 5;
   */
  rowIds: string[]; // Associated row IDs (mentioned in the issue).
  /**
   * Run IDs used to associate this issue with a particular target (in case of
   * repeats, or across runs on different dashboards).
   *
   * @generated from protobuf field: repeated string run_ids = 6;
   */
  runIds: string[];
  /**
   * Targets + methods associated with this issue.
   * Only set if test group's `link_bugs_by_test_methods` is True, else all
   * targets + methods will be linked to this issue.
   *
   * @generated from protobuf field: repeated TargetAndMethods targets_and_methods = 7;
   */
  targetsAndMethods: TargetAndMethods[];
}
/**
 * @generated from protobuf message IssueState
 */
export interface IssueState {
  /**
   * List of collected info for bugs.
   *
   * @generated from protobuf field: repeated IssueInfo issue_info = 1;
   */
  issueInfo: IssueInfo[];
}
// @generated message type with reflection information, may provide speed optimized methods
class TargetAndMethods$Type extends MessageType<TargetAndMethods> {
  constructor() {
    super('TargetAndMethods', [
      {
        no: 1,
        name: 'target_name',
        kind: 'scalar',
        T: 9 /*ScalarType.STRING*/,
      },
      {
        no: 2,
        name: 'method_names',
        kind: 'scalar',
        repeat: 2 /*RepeatType.UNPACKED*/,
        T: 9 /*ScalarType.STRING*/,
      },
    ]);
  }
  create(value?: PartialMessage<TargetAndMethods>): TargetAndMethods {
    const message = { targetName: '', methodNames: [] };
    globalThis.Object.defineProperty(message, MESSAGE_TYPE, {
      enumerable: false,
      value: this,
    });
    if (value !== undefined)
      reflectionMergePartial<TargetAndMethods>(this, message, value);
    return message;
  }
  internalBinaryRead(
    reader: IBinaryReader,
    length: number,
    options: BinaryReadOptions,
    target?: TargetAndMethods
  ): TargetAndMethods {
    let message = target ?? this.create(),
      end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string target_name */ 1:
          message.targetName = reader.string();
          break;
        case /* repeated string method_names */ 2:
          message.methodNames.push(reader.string());
          break;
        default:
          let u = options.readUnknownField;
          if (u === 'throw')
            throw new globalThis.Error(
              `Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`
            );
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(
              this.typeName,
              message,
              fieldNo,
              wireType,
              d
            );
      }
    }
    return message;
  }
  internalBinaryWrite(
    message: TargetAndMethods,
    writer: IBinaryWriter,
    options: BinaryWriteOptions
  ): IBinaryWriter {
    /* string target_name = 1; */
    if (message.targetName !== '')
      writer.tag(1, WireType.LengthDelimited).string(message.targetName);
    /* repeated string method_names = 2; */
    for (let i = 0; i < message.methodNames.length; i++)
      writer.tag(2, WireType.LengthDelimited).string(message.methodNames[i]);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(
        this.typeName,
        message,
        writer
      );
    return writer;
  }
}
/**
 * @generated MessageType for protobuf message TargetAndMethods
 */
export const TargetAndMethods = new TargetAndMethods$Type();
// @generated message type with reflection information, may provide speed optimized methods
class IssueInfo$Type extends MessageType<IssueInfo> {
  constructor() {
    super('IssueInfo', [
      { no: 1, name: 'issue_id', kind: 'scalar', T: 9 /*ScalarType.STRING*/ },
      { no: 2, name: 'title', kind: 'scalar', T: 9 /*ScalarType.STRING*/ },
      { no: 3, name: 'is_autobug', kind: 'scalar', T: 8 /*ScalarType.BOOL*/ },
      {
        no: 8,
        name: 'is_flakiness_bug',
        kind: 'scalar',
        T: 8 /*ScalarType.BOOL*/,
      },
      {
        no: 4,
        name: 'last_modified',
        kind: 'scalar',
        T: 1 /*ScalarType.DOUBLE*/,
      },
      {
        no: 5,
        name: 'row_ids',
        kind: 'scalar',
        repeat: 2 /*RepeatType.UNPACKED*/,
        T: 9 /*ScalarType.STRING*/,
      },
      {
        no: 6,
        name: 'run_ids',
        kind: 'scalar',
        repeat: 2 /*RepeatType.UNPACKED*/,
        T: 9 /*ScalarType.STRING*/,
      },
      {
        no: 7,
        name: 'targets_and_methods',
        kind: 'message',
        repeat: 1 /*RepeatType.PACKED*/,
        T: () => TargetAndMethods,
      },
    ]);
  }
  create(value?: PartialMessage<IssueInfo>): IssueInfo {
    const message = {
      issueId: '',
      title: '',
      isAutobug: false,
      isFlakinessBug: false,
      lastModified: 0,
      rowIds: [],
      runIds: [],
      targetsAndMethods: [],
    };
    globalThis.Object.defineProperty(message, MESSAGE_TYPE, {
      enumerable: false,
      value: this,
    });
    if (value !== undefined)
      reflectionMergePartial<IssueInfo>(this, message, value);
    return message;
  }
  internalBinaryRead(
    reader: IBinaryReader,
    length: number,
    options: BinaryReadOptions,
    target?: IssueInfo
  ): IssueInfo {
    let message = target ?? this.create(),
      end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string issue_id */ 1:
          message.issueId = reader.string();
          break;
        case /* string title */ 2:
          message.title = reader.string();
          break;
        case /* bool is_autobug */ 3:
          message.isAutobug = reader.bool();
          break;
        case /* bool is_flakiness_bug */ 8:
          message.isFlakinessBug = reader.bool();
          break;
        case /* double last_modified */ 4:
          message.lastModified = reader.double();
          break;
        case /* repeated string row_ids */ 5:
          message.rowIds.push(reader.string());
          break;
        case /* repeated string run_ids */ 6:
          message.runIds.push(reader.string());
          break;
        case /* repeated TargetAndMethods targets_and_methods */ 7:
          message.targetsAndMethods.push(
            TargetAndMethods.internalBinaryRead(
              reader,
              reader.uint32(),
              options
            )
          );
          break;
        default:
          let u = options.readUnknownField;
          if (u === 'throw')
            throw new globalThis.Error(
              `Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`
            );
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(
              this.typeName,
              message,
              fieldNo,
              wireType,
              d
            );
      }
    }
    return message;
  }
  internalBinaryWrite(
    message: IssueInfo,
    writer: IBinaryWriter,
    options: BinaryWriteOptions
  ): IBinaryWriter {
    /* string issue_id = 1; */
    if (message.issueId !== '')
      writer.tag(1, WireType.LengthDelimited).string(message.issueId);
    /* string title = 2; */
    if (message.title !== '')
      writer.tag(2, WireType.LengthDelimited).string(message.title);
    /* bool is_autobug = 3; */
    if (message.isAutobug !== false)
      writer.tag(3, WireType.Varint).bool(message.isAutobug);
    /* bool is_flakiness_bug = 8; */
    if (message.isFlakinessBug !== false)
      writer.tag(8, WireType.Varint).bool(message.isFlakinessBug);
    /* double last_modified = 4; */
    if (message.lastModified !== 0)
      writer.tag(4, WireType.Bit64).double(message.lastModified);
    /* repeated string row_ids = 5; */
    for (let i = 0; i < message.rowIds.length; i++)
      writer.tag(5, WireType.LengthDelimited).string(message.rowIds[i]);
    /* repeated string run_ids = 6; */
    for (let i = 0; i < message.runIds.length; i++)
      writer.tag(6, WireType.LengthDelimited).string(message.runIds[i]);
    /* repeated TargetAndMethods targets_and_methods = 7; */
    for (let i = 0; i < message.targetsAndMethods.length; i++)
      TargetAndMethods.internalBinaryWrite(
        message.targetsAndMethods[i],
        writer.tag(7, WireType.LengthDelimited).fork(),
        options
      ).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(
        this.typeName,
        message,
        writer
      );
    return writer;
  }
}
/**
 * @generated MessageType for protobuf message IssueInfo
 */
export const IssueInfo = new IssueInfo$Type();
// @generated message type with reflection information, may provide speed optimized methods
class IssueState$Type extends MessageType<IssueState> {
  constructor() {
    super('IssueState', [
      {
        no: 1,
        name: 'issue_info',
        kind: 'message',
        repeat: 1 /*RepeatType.PACKED*/,
        T: () => IssueInfo,
      },
    ]);
  }
  create(value?: PartialMessage<IssueState>): IssueState {
    const message = { issueInfo: [] };
    globalThis.Object.defineProperty(message, MESSAGE_TYPE, {
      enumerable: false,
      value: this,
    });
    if (value !== undefined)
      reflectionMergePartial<IssueState>(this, message, value);
    return message;
  }
  internalBinaryRead(
    reader: IBinaryReader,
    length: number,
    options: BinaryReadOptions,
    target?: IssueState
  ): IssueState {
    let message = target ?? this.create(),
      end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* repeated IssueInfo issue_info */ 1:
          message.issueInfo.push(
            IssueInfo.internalBinaryRead(reader, reader.uint32(), options)
          );
          break;
        default:
          let u = options.readUnknownField;
          if (u === 'throw')
            throw new globalThis.Error(
              `Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`
            );
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(
              this.typeName,
              message,
              fieldNo,
              wireType,
              d
            );
      }
    }
    return message;
  }
  internalBinaryWrite(
    message: IssueState,
    writer: IBinaryWriter,
    options: BinaryWriteOptions
  ): IBinaryWriter {
    /* repeated IssueInfo issue_info = 1; */
    for (let i = 0; i < message.issueInfo.length; i++)
      IssueInfo.internalBinaryWrite(
        message.issueInfo[i],
        writer.tag(1, WireType.LengthDelimited).fork(),
        options
      ).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(
        this.typeName,
        message,
        writer
      );
    return writer;
  }
}
/**
 * @generated MessageType for protobuf message IssueState
 */
export const IssueState = new IssueState$Type();
