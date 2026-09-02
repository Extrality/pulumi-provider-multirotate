import * as pulumi from "@pulumi/pulumi";
/**
 * Rotates credentials. It maintains `timestamps` and an `index` into them; when the timestamp at `index` is older than `rotationPeriodDays`, the cursor advances to the next slot and that slot is re-stamped with the current time.
 */
export declare class MultiRotate extends pulumi.CustomResource {
    /**
     * Get an existing MultiRotate resource's state with the given name, ID, and optional extra
     * properties used to qualify the lookup.
     *
     * @param name The _unique_ name of the resulting resource.
     * @param id The _unique_ provider ID of the resource to lookup.
     * @param opts Optional settings to control the behavior of the CustomResource.
     */
    static get(name: string, id: pulumi.Input<pulumi.ID>, opts?: pulumi.CustomResourceOptions): MultiRotate;
    /**
     * Returns true if the given object is an instance of MultiRotate.  This is designed to work even
     * when multiple copies of the Pulumi SDK have been loaded into the same process.
     */
    static isInstance(obj: any): obj is MultiRotate;
    /**
     * Number of timestamps in the rotation window.
     */
    readonly count: pulumi.Output<number>;
    /**
     * The timestamp at `timestamps[index]`.
     */
    readonly currentTimestamp: pulumi.Output<string>;
    /**
     * Index of the last rotated timestamp.
     */
    readonly index: pulumi.Output<number>;
    /**
     * Number of days a timestamp stays valid before it is rotated.
     */
    readonly rotationPeriodDays: pulumi.Output<number>;
    /**
     * The rotation window: `count` ISO-8601 timestamps.
     */
    readonly timestamps: pulumi.Output<string[]>;
    /**
     * Create a MultiRotate resource with the given unique name, arguments, and options.
     *
     * @param name The _unique_ name of the resource.
     * @param args The arguments to use to populate this resource's properties.
     * @param opts A bag of options that control this resource's behavior.
     */
    constructor(name: string, args?: MultiRotateArgs, opts?: pulumi.CustomResourceOptions);
}
/**
 * The set of arguments for constructing a MultiRotate resource.
 */
export interface MultiRotateArgs {
    /**
     * Number of timestamps in the rotation window.
     */
    count?: pulumi.Input<number | undefined>;
    /**
     * Number of days a timestamp stays valid before it is rotated.
     */
    rotationPeriodDays?: pulumi.Input<number | undefined>;
}
